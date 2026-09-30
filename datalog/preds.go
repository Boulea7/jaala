package datalog

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// This file is the single positive primitive of the evaluator: extendAtom. Every callable relation —
// a fact relation (EDB), a rule-derived relation (IDB), or a computed built-in (a filter, or a
// host-supplied generator) — is "given a partial binding, yield zero-or-more extended bindings."
// solve() drives the positive body through extendAtom; negation reuses the SAME primitive (atomHolds
// runs extendAtom and asks only whether it yields anything — negation as failure). That unification
// is why there is one dispatch, not one per kind, and why `not R(...)` works uniformly for EDB, IDB,
// filters, and generators.

// A Builtin is a predicate the engine computes rather than looks up. It is exactly one of two kinds.
//
// A FILTER (Holds set) tests arguments that must all be bound, keeping a binding when Holds is true.
// Because negation runs the same test and asks whether it yielded, `not name(...)` keeps a binding
// exactly when Holds is false, so the two directions cannot disagree. An unbound argument is an
// error, the same shape as an unbound comparison operand.
//
// A GENERATOR (Gen set) PRODUCES values, enumerating solutions from wherever the host keeps them.
// That is the property that makes clause order matter: a generator whose own input is unbound
// enumerates from every candidate, so appearing first in a rule body is a whole-dataset scan no later
// literal can undo (see GeneratorFirstRules). A generator must also only ever emit values drawn from
// finite host data, or evaluation stops being guaranteed to terminate.
type Builtin struct {
	// Arity is the argument count, or the minimum when MaxArity is set.
	Arity int
	// MaxArity, when non-zero, is the inclusive maximum argument count. It exists so an optional
	// trailing argument is admitted by the POSITIVE path and the NEGATION path through one test
	// (accepts) rather than two length checks that could drift.
	MaxArity int
	// Holds is a filter's test over its (all-bound) argument values.
	Holds func(args []Value) (bool, error)
	// Gen is a generator's enumeration. args holds each argument's value where the binding fixes
	// it. Gen calls emit once per solution with a value for EVERY argument, plus the citations that
	// justify it; the engine unifies each solution against the atom, so one that disagrees with a
	// bound argument is dropped there rather than in Gen. src is the Base's Source, for a generator
	// that reads the host's data through it.
	//
	// The engine copies what it needs out of vals and cites before emit returns and never keeps
	// either slice, so a generator may reuse one buffer for every solution it emits.
	Gen func(src Source, args []Arg, emit func(vals []Value, cites []string) error) error
}

// An Arg is one argument as a generator sees it: its value when the binding fixes it.
type Arg struct {
	Value Value
	Bound bool
}

// generator reports whether the builtin produces values rather than filtering them.
func (bi Builtin) generator() bool { return bi.Gen != nil }

// accepts reports whether n is a valid argument count for this builtin.
func (bi Builtin) accepts(n int) bool {
	if bi.MaxArity == 0 {
		return n == bi.Arity
	}
	return n >= bi.Arity && n <= bi.MaxArity
}

// arityLabel renders the accepted arity for an error message ("2" or "2 or 3").
func (bi Builtin) arityLabel() string {
	if bi.MaxArity == 0 {
		return fmt.Sprintf("%d", bi.Arity)
	}
	return fmt.Sprintf("%d or %d", bi.Arity, bi.MaxArity)
}

// Filter builds a filter Builtin from a boolean over its all-bound argument values.
func Filter(arity int, holds func(args []Value) (bool, error)) Builtin {
	return Builtin{Arity: arity, Holds: holds}
}

// Predicates is the set of computed predicates a Base can call, keyed by name. It is a value a host
// composes and hands to NewBase, not ambient state, so two hosts in one process can offer different
// vocabularies.
type Predicates struct {
	m map[string]Builtin
}

// NewPredicates returns an empty predicate set.
func NewPredicates() *Predicates { return &Predicates{m: map[string]Builtin{}} }

// StandardPredicates returns the filters every host gets unless it composes its own set:
// contains, prefix, suffix, glob, match and absent.
func StandardPredicates() *Predicates {
	p := NewPredicates()
	p.Add("contains", strFilter(strings.Contains))
	p.Add("prefix", strFilter(strings.HasPrefix))
	p.Add("suffix", strFilter(strings.HasSuffix))
	p.Add("glob", patFilter(CompileGlob))
	p.Add("match", patFilter(CompilePattern))
	// absent(?x) is the only way to ASK about a field the source did not state. Before Value.Absent
	// existed such a field bound to the empty string, so it was not merely hard to select, it was
	// indistinguishable from one that was stated as "". Its negation is the useful half as often as
	// not: `not absent(?min)` reads "this row states a lower bound".
	p.Add("absent", Filter(1, func(args []Value) (bool, error) { return args[0].Absent, nil }))
	return p
}

// Add registers a predicate. It panics on an empty name, a builtin that is neither or both a filter
// and a generator, a filter with arity below one, or a name already present, because each is a
// programming error that must fail loudly at load rather than silently at query time (the same
// contract as net/http.Handle).
func (p *Predicates) Add(name string, b Builtin) {
	if name == "" {
		panic("datalog: Predicates.Add with empty name")
	}
	if (b.Holds == nil) == (b.Gen == nil) {
		panic(fmt.Sprintf("datalog: Predicates.Add(%q) needs exactly one of Holds and Gen", name))
	}
	if b.Holds != nil && b.Arity < 1 {
		panic(fmt.Sprintf("datalog: Predicates.Add(%q) needs arity >= 1", name))
	}
	if _, ok := p.m[name]; ok {
		panic(fmt.Sprintf("datalog: Predicates.Add(%q) collides with a predicate already present", name))
	}
	p.m[name] = b
}

// Clone returns an independent copy, so a caller can add predicates without changing the set it was
// copied from. A test registering a predicate uses it to leave the shared set as it found it.
func (p *Predicates) Clone() *Predicates {
	out := NewPredicates()
	if p != nil {
		for k, v := range p.m {
			out.m[k] = v
		}
	}
	return out
}

// Has reports whether name is a registered predicate.
func (p *Predicates) Has(name string) bool {
	_, ok := p.lookup(name)
	return ok
}

// Names returns every registered predicate name, sorted.
func (p *Predicates) Names() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.m))
	for n := range p.m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (p *Predicates) lookup(name string) (Builtin, bool) {
	if p == nil {
		return Builtin{}, false
	}
	b, ok := p.m[name]
	return b, ok
}

// strFilter wraps a string(value, pattern) bool as a 2-arity filter (the shape of
// contains/prefix/suffix).
func strFilter(fn func(s, pat string) bool) Builtin {
	return Filter(2, func(args []Value) (bool, error) { return fn(args[0].S, args[1].S), nil })
}

// patFilter is strFilter for the two PATTERN predicates (glob, match): the pattern must be compiled
// before it can be tested, so a malformed one is an EVAL ERROR rather than a non-match. That
// direction matters — a bad pattern that quietly matched nothing would read as "the data is clean"
// on a completeness check.
func patFilter(compile func(string) (*regexp.Regexp, error)) Builtin {
	return Filter(2, func(args []Value) (bool, error) {
		re, err := compile(args[1].S)
		if err != nil {
			return false, err
		}
		return re.MatchString(args[0].S), nil
	})
}

// extendBuiltin runs a builtin under the current binding.
func extendBuiltin(bi Builtin, atom *Atom, bnd *binding, b *Base, yield func(*binding) error) error {
	if bi.Holds != nil {
		args := make([]Value, len(atom.Args))
		for i, a := range atom.Args {
			v, ok := resolve(a, bnd)
			if !ok {
				return fmt.Errorf("query: %s needs all arguments bound (a variable must appear in a relation before %s tests it)", atom.Relation, atom.Relation)
			}
			args[i] = v
		}
		ok, err := bi.Holds(args)
		if err != nil {
			return err
		}
		if ok {
			return yield(bnd)
		}
		return nil
	}
	args := make([]Arg, len(atom.Args))
	for i, a := range atom.Args {
		v, ok := resolve(a, bnd)
		args[i] = Arg{Value: v, Bound: ok}
	}
	return bi.Gen(b.src, args, func(vals []Value, cites []string) error {
		if len(vals) != len(atom.Args) {
			return fmt.Errorf("query: internal: %s emitted %d values for %d arguments", atom.Relation, len(vals), len(atom.Args))
		}
		next := bnd.clone()
		for j, arg := range atom.Args {
			if !bindArg(next, arg, vals[j]) {
				return nil
			}
		}
		next.cites = append(next.cites, cites...)
		return yield(next)
	})
}

// extendAtom is the single positive primitive: it yields every binding that satisfies atom as an
// extension of bnd. Dispatch is by relation kind — a computed built-in, an EDB fact relation, or an
// IDB rule relation — each checked for arity first so a wrong-arity atom fails clearly. yield is
// called per solution and its error (from a deeper solve, or the negation early-stop) propagates.
func (b *Base) extendAtom(atom *Atom, bnd *binding, yield func(*binding) error) error {
	if err := b.checkAtom(atom); err != nil {
		return err
	}
	rel := atom.Relation
	if bi, ok := b.preds.lookup(rel); ok {
		return extendBuiltin(bi, atom, bnd, b, yield)
	}
	if _, ok := b.schemaOf(rel); ok {
		return b.extendEDB(atom, bnd, yield)
	}
	return b.extendIDB(atom, bnd, yield)
}

// schemaOf is the Source's schema for rel, or false when there is no Source.
func (b *Base) schemaOf(rel string) (Schema, bool) {
	if b.src == nil {
		return Schema{}, false
	}
	return b.src.Schema(rel)
}

// checkArgValues rejects a CONSTANT naming a value its argument cannot hold, for the arguments whose
// values are a vocabulary the Source defines. A variable is unaffected, and a relation declaring no
// domain is unchanged.
//
// It exists because the alternative is silence: a misspelled constant matches nothing and answers
// "no results", which reads as a fact about the data rather than a typo. An empty answer to a
// question that was never valid is the worst available outcome.
func (b *Base) checkArgValues(atom *Atom, s Schema) error {
	if len(s.Domains) == 0 {
		return nil
	}
	for i, arg := range atom.Args {
		if arg.Const == nil || i >= len(s.Labels) || i >= len(s.Domains) {
			continue
		}
		label := s.Labels[i]
		allowed := s.Domains[i]
		if len(allowed) == 0 {
			continue
		}
		got := arg.Const.S
		if slices.Contains(allowed, got) {
			continue
		}
		return fmt.Errorf("query: %s's %q argument cannot be %q%s (it holds one of: %s)",
			atom.Relation, label, got, didYouMeanValue(allowed, got), strings.Join(allowed, ", "))
	}
	return nil
}

// checkAtom reports whether an atom names something the evaluator can read, at an arity that relation
// accepts. It is the three-way dispatch's precondition, split out so Validate can apply it to EVERY
// atom without evaluating.
//
// Splitting it matters: solving stops as soon as an atom yields nothing, so a wrong-arity atom LATER
// in a body is never reached on data where an earlier one matches nothing. Checking arity only where
// a solve happens to arrive is checking it sometimes.
func (b *Base) checkAtom(atom *Atom) error {
	rel := atom.Relation
	if bi, ok := b.preds.lookup(rel); ok {
		if !bi.accepts(len(atom.Args)) {
			return fmt.Errorf("query: %s takes %s args, got %d", rel, bi.arityLabel(), len(atom.Args))
		}
		return nil
	}
	if s, ok := b.schemaOf(rel); ok {
		if len(atom.Args) != s.Arity {
			return fmt.Errorf("query: relation %q takes %d args, got %d", rel, s.Arity, len(atom.Args))
		}
		return b.checkArgValues(atom, s)
	}
	if b.isIDB(rel) {
		if len(atom.Args) != b.idbArity[rel] {
			return fmt.Errorf("query: relation %q takes %d args, got %d", rel, b.idbArity[rel], len(atom.Args))
		}
		return nil
	}
	return fmt.Errorf("query: unknown relation %q%s", rel, b.didYouMean(rel))
}

// extendEDB fans an EDB atom over the tuples of its relation, unifying each into the binding.
//
// When the binding already fixes some of the atom's arguments, the candidates come from an index on
// exactly those positions instead of from the whole relation. unify still decides every candidate,
// so the index only ever has to avoid MISSING a match; see index.go.
func (b *Base) extendEDB(atom *Atom, bnd *binding, yield func(*binding) error) error {
	rows := b.edbTuples(atom.Relation)
	pos, all := b.edbCandidates(atom, rows, bnd)
	for i := 0; ; i++ {
		var t Tuple
		if all {
			if i >= len(rows) {
				break
			}
			t = rows[i]
		} else {
			if i >= len(pos) {
				break
			}
			t = rows[pos[i]]
		}
		b.countWork()
		if next, ok := unify(atom.Args, t, bnd); ok {
			if err := yield(next); err != nil {
				return err
			}
		}
	}
	return nil
}

// extendIDB fans a rule-defined atom over the derived tuples of its relation, carrying each tuple's
// provenance forward — the same shape as extendEDB, over the materialized IDB store.
func (b *Base) extendIDB(atom *Atom, bnd *binding, yield func(*binding) error) error {
	for _, t := range b.idbCandidates(atom, bnd) {
		b.countWork()
		out := bnd.clone()
		ok := true
		for j, arg := range atom.Args {
			if !bindArg(out, arg, t.vals[j]) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out.cites = append(out.cites, t.cites...)
		if err := yield(out); err != nil {
			return err
		}
	}
	return nil
}

// errStop unwinds extendAtom after the first yield — negation only needs existence, not enumeration.
var errStop = errors.New("query: stop")

// atomHolds reports whether atom has any solution under bnd (negation as failure): `not atom` holds
// exactly when this is false. It runs the same extendAtom the positive solve uses, stopping at the
// first match. A malformed atom (an unbound filter argument, wrong arity) surfaces as an error rather
// than silently reading as "no match".
func (b *Base) atomHolds(atom *Atom, bnd *binding) (bool, error) {
	found := false
	err := b.extendAtom(atom, bnd, func(*binding) error { found = true; return errStop })
	if err != nil && err != errStop {
		return false, err
	}
	return found, nil
}

// arityAccepts reports whether n is a valid argument count for any callable relation — built-in, EDB,
// or IDB — and whether the relation exists at all. It is the SAME admission test extendAtom applies
// on the positive path, so a variadic built-in cannot be accepted in a positive atom while its
// negation is rejected.
func (b *Base) arityAccepts(rel string, n int) (ok bool, known bool) {
	if bi, found := b.preds.lookup(rel); found {
		return bi.accepts(n), true
	}
	if s, found := b.schemaOf(rel); found {
		return n == s.Arity, true
	}
	if ar, found := b.idbArity[rel]; found {
		return n == ar, true
	}
	return false, false
}

// arityLabelOf renders a relation's accepted argument count for an error message.
func (b *Base) arityLabelOf(rel string) string {
	if bi, ok := b.preds.lookup(rel); ok {
		return bi.arityLabel()
	}
	if s, ok := b.schemaOf(rel); ok {
		return fmt.Sprintf("%d", s.Arity)
	}
	return fmt.Sprintf("%d", b.idbArity[rel])
}
