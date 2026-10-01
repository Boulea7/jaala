package ns

import (
	"fmt"
	"regexp"
	"strings"
)

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
	// (Accepts) rather than two length checks that could drift.
	MaxArity int
	// Holds is a filter's test over its (all-bound) argument values.
	Holds func(args []Value) (bool, error)
	// Labels names the arguments, and Types says what each denotes, exactly as for a base relation's
	// Schema. Both are optional; they are what a host lists and what column kinds are read from.
	Labels []string
	Types  []ArgType
	// Doc is a one-line description a host shows when the predicate is listed.
	Doc string
	// Gen is a generator's enumeration. args holds each argument's value where the binding fixes
	// it. Gen calls emit once per solution with a value for EVERY argument, plus the citations that
	// justify it; the engine unifies each solution against the atom, so one that disagrees with a
	// bound argument is dropped there rather than in Gen. src is the Source the query runs over, for a generator
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

// IsGenerator reports whether the builtin produces values rather than filtering them.
func (bi Builtin) IsGenerator() bool { return bi.Gen != nil }

// Accepts reports whether n is a valid argument count for this builtin.
func (bi Builtin) Accepts(n int) bool {
	if bi.MaxArity == 0 {
		return n == bi.Arity
	}
	return n >= bi.Arity && n <= bi.MaxArity
}

// ArityLabel renders the accepted arity for an error message ("2" or "2 or 3").
func (bi Builtin) ArityLabel() string {
	if bi.MaxArity == 0 {
		return fmt.Sprintf("%d", bi.Arity)
	}
	return fmt.Sprintf("%d or %d", bi.Arity, bi.MaxArity)
}

// Filter builds a filter Builtin from a boolean over its all-bound argument values.
func Filter(arity int, holds func(args []Value) (bool, error)) Builtin {
	return Builtin{Arity: arity, Holds: holds}
}

// StandardPredicates registers the predicates every host gets unless it composes its own set: the
// string tests str.contains, str.prefix, str.suffix, str.glob and str.match, and absent at the root.
// absent is not a string test (it asks whether a field was stated at all, for any value), which is
// why it stays out of str. A host that wants other names registers these builtins itself.
func StandardPredicates(r *Vocabulary) error {
	for _, p := range []struct {
		path string
		b    Builtin
	}{
		{"str.contains", strFilter(strings.Contains, "substring", "reports whether a string contains a substring")},
		{"str.prefix", strFilter(strings.HasPrefix, "prefix", "reports whether a string starts with a prefix")},
		{"str.suffix", strFilter(strings.HasSuffix, "suffix", "reports whether a string ends with a suffix")},
		{"str.glob", patFilter(CompileGlob, "pattern", "reports whether a string matches a glob pattern")},
		{"str.match", patFilter(CompilePattern, "regex", "reports whether a string matches a regular expression")},
		// absent(?x) is the only way to ASK about a field the source did not state. Before
		// Value.Absent existed such a field bound to the empty string, so it was not merely hard to
		// select, it was indistinguishable from one that was stated as "". Its negation is the useful
		// half as often as not: `not absent(?min)` reads "this row states a lower bound".
		{"absent", Builtin{
			Arity: 1, Labels: []string{"value"},
			Doc:   "reports whether the source left a field unstated, for any value",
			Holds: func(args []Value) (bool, error) { return args[0].Absent, nil },
		}},
	} {
		if err := r.AddPredicate(p.path, p.b); err != nil {
			return err
		}
	}
	return nil
}

// strFilter wraps a string(value, pattern) bool as a 2-arity filter (the shape of
// str.contains/str.prefix/str.suffix).
func strFilter(fn func(s, pat string) bool, second, doc string) Builtin {
	b := Filter(2, func(args []Value) (bool, error) { return fn(args[0].S, args[1].S), nil })
	return stringTest(b, second, doc)
}

// stringTest labels a two-argument string test and types both arguments as strings.
func stringTest(b Builtin, second, doc string) Builtin {
	b.Labels = []string{"string", second}
	b.Types = []ArgType{{Type: TypeString}, {Type: TypeString}}
	b.Doc = doc
	return b
}

// patFilter is strFilter for the two PATTERN predicates (str.glob, str.match): the pattern must be compiled
// before it can be tested, so a malformed one is an EVAL ERROR rather than a non-match. That
// direction matters — a bad pattern that quietly matched nothing would read as "the data is clean"
// on a completeness check.
func patFilter(compile func(string) (*regexp.Regexp, error), second, doc string) Builtin {
	return stringTest(Filter(2, func(args []Value) (bool, error) {
		re, err := compile(args[1].S)
		if err != nil {
			return false, err
		}
		return re.MatchString(args[0].S), nil
	}), second, doc)
}
