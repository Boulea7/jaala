package datalog

import (
	"context"
	"math"
	"sort"
)

// SemiNaive is the evaluator a host should run. It answers exactly as Naive does, deriving rules by
// a semi-naive fixpoint instead of a naive one.
//
// A naive round re-runs every rule over everything derived so far, so on a chain of n nodes a
// transitive closure takes about n rounds of about n² derivations each, most of them repeats: the
// work grows with n³. A derivation that uses only facts older than the last round was already made in
// an earlier round, so a semi-naive round only runs the derivations that use at least one fact new in
// the last round (the delta). Each fact is joined against once rather than once per round, and the
// same closure grows with about n².
//
// It reads a derived relation the Base already holds, from an earlier query that evaluated it in full,
// instead of deriving it (see reuseDerived). It also inlines single-rule, non-recursive derived relations into their callers (see unfold), so a
// bound argument reaches the literals that can use it; rewrites the derived relations still called
// with bound arguments so only what the query demands is derived (see magic, and factor for
// right-linear recursion); and then plans each rule body and the goal before evaluating (see plan): a
// literal runs once as much as possible is bound, a comparison or filter as soon as its variables
// are, and a generator once one of its Modes is satisfied. That makes cost independent of how a body is written, at the price of
// citations: a different join order can make a different derivation of a tuple the first, so a planned
// answer's rows match Naive's while a row's citations may come from another valid derivation.
type SemiNaive struct {
	// WrittenOrder runs bodies as written, with none of the rewrites (inlining, demand, planning).
	// Its answers then match Naive's citations too, which is what the tests compare it on.
	WrittenOrder bool
}

// Eval answers the query as Naive.Eval does.
func (s SemiNaive) Eval(ctx context.Context, q Query, b *Base, opts ...Option) ([]Row, error) {
	var rewrite func(*Base, Query) Query
	if !s.WrittenOrder {
		rewrite = func(b *Base, q Query) Query {
			q = reuseDerived(b, withoutUnreached(q))
			if b.witnessing() {
				return plan(b, magic(b, q)) // inlining would remove a relation's node from the witness
			}
			return plan(b, magic(b, unfold(b, q)))
		}
	}
	return evaluate(ctx, q, b, opts, rewrite, s.materialize)
}

// deltaSep marks the relation holding a stratum relation's delta: the tuples it gained in the last
// round. The parser never accepts it in a relation name, so no query can read a delta.
const deltaSep = "\x00delta"

// materialize is SemiNaive's fixpoint: it derives the rules stratum by stratum, as Naive's does, and
// within a stratum component by component (see components), with a semi-naive fixpoint in each
// recursive one. A relation that does not read itself, even through others, is derived once.
//
// In a component, round zero runs every rule in full. After that a rule matters only if its body reads
// a relation of the same component (everything else it reads is complete already), and for such a rule
// each round runs one variant per such atom, with that atom reading the delta and every other atom
// reading the whole relation. A new tuple's derivation uses some fact of the component that was new
// last round, at the latest, so one of the variants finds it; the fixpoint is reached when a round
// adds nothing. Unless WrittenOrder is set, a variant starts from its
// delta when the delta is the smaller side (see variant.pick).
//
// The delta is installed as an ordinary derived relation under a name no query can spell, so solving,
// indexing and negation are untouched: a variant is the rule with one atom renamed. Negation never
// reads the stratum's own relations (stratification forbids it), so only positive atoms vary.
func (s SemiNaive) materialize(b *Base, rules []Rule) error {
	byHead, strata, err := b.checkRules(rules)
	if err != nil {
		return err
	}
	for _, level := range strata {
		for _, comp := range components(level, byHead) {
			if err := s.fixpoint(b, byHead, comp); err != nil {
				return err
			}
		}
	}
	return nil
}

// fixpoint derives one component of a stratum (see materialize).
func (s SemiNaive) fixpoint(b *Base, byHead map[string][]Rule, comp []string) error {
	in := make(map[string]bool, len(comp))
	for _, rel := range comp {
		in[rel] = true
	}
	// The delta variants are made once per component: one per rule and recursive atom, in the
	// planned order and, unless that already starts at the delta, a second one that does (see
	// deltaFirst). Each round runs whichever starts from fewer tuples.
	var variants []variant
	for _, rel := range comp {
		for _, r := range byHead[rel] {
			for i, lit := range r.Body.Literals {
				if lit.Pos == nil || !in[lit.Pos.Relation] {
					continue
				}
				v := variant{reads: lit.Pos.Relation, rule: readingDelta(r, i)}
				if !s.WrittenOrder && i > 0 {
					first := deltaFirst(b, v.rule, i)
					v.first = &first
				}
				variants = append(variants, v)
			}
		}
	}
	mark := marks(b, comp)
	for _, rel := range comp {
		for _, r := range byHead[rel] {
			if err := derive(b, r); err != nil {
				return err
			}
		}
	}
	rounds := 1
	if b.run.explain != nil {
		defer func() {
			for _, rel := range comp {
				b.run.explain.rounds[rel] = rounds
			}
		}()
	}
	if len(variants) == 0 {
		return nil // not recursive: one pass derived everything
	}
	delta := since(b, comp, mark)
	for len(delta) > 0 {
		rounds++
		if err := b.run.done(); err != nil {
			return err
		}
		installDeltas(b, comp, delta)
		mark = marks(b, comp)
		for _, v := range variants {
			if len(delta[v.reads]) == 0 {
				continue
			}
			r, err := v.pick(b, len(delta[v.reads]))
			if err != nil {
				return err
			}
			if err := derive(b, r); err != nil {
				return err
			}
		}
		delta = since(b, comp, mark)
	}
	dropDeltas(b, comp)
	return nil
}

// components splits a stratum into its strongly connected components, the sets of relations that read
// one another, directly or through others, ordered so a component comes after every one it reads.
// Stratification only separates relations by negation, so a stratum mixes recursion with plain
// dependencies; deriving it as one fixpoint would count everything a relation read as new in the
// round after it was derived, and run the reading rules over it again (#51).
func components(stratum []string, byHead map[string][]Rule) [][]string {
	in := make(map[string]bool, len(stratum))
	for _, rel := range stratum {
		in[rel] = true
	}
	reads := map[string][]string{}
	for _, rel := range stratum {
		seen := map[string]bool{}
		for _, r := range byHead[rel] {
			for _, lit := range r.Body.Literals {
				if lit.Pos != nil && in[lit.Pos.Relation] && !seen[lit.Pos.Relation] {
					seen[lit.Pos.Relation] = true
					reads[rel] = append(reads[rel], lit.Pos.Relation)
				}
			}
		}
		sort.Strings(reads[rel])
	}
	// Tarjan's algorithm: it completes a component only after every component reachable from it,
	// which, with edges from a relation to what it reads, is dependency order.
	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var out [][]string
	var visit func(string)
	visit = func(rel string) {
		index[rel], low[rel] = len(index), len(index)
		stack = append(stack, rel)
		onStack[rel] = true
		for _, next := range reads[rel] {
			if _, done := index[next]; !done {
				visit(next)
				low[rel] = min(low[rel], low[next])
			} else if onStack[next] {
				low[rel] = min(low[rel], index[next])
			}
		}
		if low[rel] != index[rel] {
			return
		}
		var comp []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			comp = append(comp, top)
			if top == rel {
				break
			}
		}
		sort.Strings(comp)
		out = append(out, comp)
	}
	for _, rel := range stratum {
		if _, done := index[rel]; !done {
			visit(rel)
		}
	}
	return out
}

// A variant is a recursive rule with one same-stratum atom reading its relation's delta.
type variant struct {
	reads string // the relation whose delta it reads
	rule  Rule   // in the planned order
	first *Rule  // starting from the delta, when the planned order does not; nil under WrittenOrder
}

// pick is the order to run a variant in this round, given its delta's size: starting from the delta
// when that is fewer tuples than the planned order's first literal would scan, else the planned order.
// A delta is usually a handful of tuples, but not always: when the first round derives almost
// everything (a chain whose edges happen to be stored in walk order closes in one pass), the next
// round's delta is the whole relation, and probing it from a small base relation is the cheaper way
// round.
func (v variant) pick(b *Base, delta int) (Rule, error) {
	if v.first == nil {
		return v.rule, nil
	}
	n, err := scanSize(b, v.rule.Body.Literals[0])
	if err != nil {
		return Rule{}, err
	}
	if delta < n {
		return *v.first, nil
	}
	return v.rule, nil
}

// scanSize is how many tuples a body's first literal yields candidates from, with nothing bound yet
// but its constants. A comparison or a host predicate has no size to compare a delta with, so it
// counts as unbounded and the delta goes first.
func scanSize(b *Base, lit Literal) (int, error) {
	if lit.Pos == nil {
		return math.MaxInt, nil
	}
	if _, ok := b.reg.Predicate(lit.Pos.Relation); ok {
		return math.MaxInt, nil
	}
	if _, ok := b.schemaOf(lit.Pos.Relation); ok {
		rows, err := b.edbTuples(lit.Pos.Relation)
		if err != nil {
			return 0, err
		}
		if pos, all := b.edbCandidates(lit.Pos, rows, newBinding()); !all {
			return len(pos), nil
		}
		return len(rows), nil
	}
	return len(b.idbCandidates(lit.Pos, newBinding())), nil
}

// deltaFirst orders a delta variant to start from its delta atom (at body position i), then plans the
// rest from what that binds. The delta is what is new since the last round, usually a handful of
// tuples, so starting there makes a round's work follow the delta rather than the largest relation in
// the body: a rule written edge(?x, ?y), r(?x) otherwise scans every edge each round to find the few
// whose ?x is new.
func deltaFirst(b *Base, r Rule, i int) Rule {
	lits := r.Body.Literals
	d := lits[i]
	rest := make([]Literal, 0, len(lits)-1)
	rest = append(rest, lits[:i]...)
	rest = append(rest, lits[i+1:]...)
	entry := map[Var]bool{}
	bindAll(d.Pos, entry)
	r.Body = Body{Literals: append([]Literal{d}, planBody(b, Body{Literals: rest}, entry).Literals...)}
	return r
}

// derive applies one rule. A magic relation's tuples (see magic) record what a query asked for, not
// what produced an answer, so they are kept without citations: an adorned rule reading its magic guard
// must not pass the facts that worked out the demand on to the answer.
func derive(b *Base, r Rule) error {
	if _, err := b.applyRule(r); err != nil {
		return err
	}
	if isMagic(r.Head.Relation) {
		for i := range b.idb[r.Head.Relation] {
			b.idb[r.Head.Relation][i].cites = nil
		}
	}
	return nil
}

// marks records how many tuples each relation of a stratum holds, so since can tell what a round added.
func marks(b *Base, stratum []string) map[string]int {
	m := make(map[string]int, len(stratum))
	for _, rel := range stratum {
		m[rel] = len(b.idb[rel])
		delete(b.run.revised, rel)
	}
	return m
}

// since returns, per relation, the tuples added after mark, leaving out relations that gained none.
// A derived relation only ever grows by appending, so they are the tail of its slice. Under
// CanonicalCites a tuple older than the mark that a later derivation replaced is new as well, so what
// was derived from it is derived again.
func since(b *Base, stratum []string, mark map[string]int) map[string][]idbTuple {
	out := map[string][]idbTuple{}
	for _, rel := range stratum {
		if tuples := b.idb[rel]; len(tuples) > mark[rel] {
			out[rel] = tuples[mark[rel]:]
		}
		seen := map[int]bool{}
		for _, i := range b.run.revised[rel] {
			if i < mark[rel] && !seen[i] {
				seen[i] = true
				out[rel] = append(out[rel][:len(out[rel]):len(out[rel])], b.idb[rel][i])
			}
		}
	}
	return out
}

// installDeltas makes each relation's delta readable under its delta name for the coming round. A
// delta is replaced wholesale each round rather than appended to, so any index built over the last
// one is dropped with it.
func installDeltas(b *Base, stratum []string, delta map[string][]idbTuple) {
	dropDeltas(b, stratum)
	for _, rel := range stratum {
		d := rel + deltaSep
		b.idb[d] = append([]idbTuple(nil), delta[rel]...)
		b.idbArity[d] = b.idbArity[rel]
	}
}

// dropDeltas removes a stratum's delta relations and their indexes.
func dropDeltas(b *Base, stratum []string) {
	for _, rel := range stratum {
		d := rel + deltaSep
		delete(b.idb, d)
		delete(b.idbArity, d)
		for k := range b.idbIdx {
			if k.rel == d {
				delete(b.idbIdx, k)
			}
		}
	}
}

// readingDelta is rule r with the atom at body position i reading its relation's delta.
func readingDelta(r Rule, i int) Rule {
	lits := append([]Literal(nil), r.Body.Literals...)
	a := *lits[i].Pos
	a.Relation += deltaSep
	lits[i] = Literal{Pos: &a, at: lits[i].at}
	return Rule{Head: r.Head, Body: Body{Literals: lits}, Hops: r.Hops, HeadTypes: r.HeadTypes, text: r.text}
}
