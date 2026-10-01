package datalog

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
// It also plans each rule body and the goal before evaluating (see plan): a literal runs once as much
// as possible is bound, a comparison or filter as soon as its variables are, and a generator once one
// of its Modes is satisfied. That makes cost independent of how a body is written, at the price of
// citations: a different join order can make a different derivation of a tuple the first, so a planned
// answer's rows match Naive's while a row's citations may come from another valid derivation.
type SemiNaive struct {
	// WrittenOrder runs bodies as written instead of planning them. Its answers then match Naive's
	// citations too, which is what the tests compare it on.
	WrittenOrder bool
}

// Eval answers the query as Naive.Eval does.
func (s SemiNaive) Eval(q Query, b *Base) ([]Row, error) {
	var rewrite func(*Base, Query) Query
	if !s.WrittenOrder {
		rewrite = plan
	}
	return evaluate(q, b, rewrite, s.materialize)
}

// deltaSep marks the relation holding a stratum relation's delta: the tuples it gained in the last
// round. The parser never accepts it in a relation name, so no query can read a delta.
const deltaSep = "\x00delta"

// materialize is SemiNaive's fixpoint: it derives the rules stratum by stratum, as Naive's does, with
// a semi-naive fixpoint in each.
//
// In a stratum, round zero runs every rule in full. After that a rule matters only if its body reads
// a relation of the same stratum (a rule reading only lower strata has seen all its inputs already),
// and for such a rule each round runs one variant per same-stratum atom, with that atom reading the
// delta and every other atom reading the whole relation. A new tuple's derivation uses some
// same-stratum fact that was new last round, at the latest, so one of the variants finds it; the
// fixpoint is reached when a round adds nothing.
//
// The delta is installed as an ordinary derived relation under a name no query can spell, so solving,
// indexing and negation are untouched: a variant is the rule with one atom renamed. Negation never
// reads the stratum's own relations (stratification forbids it), so only positive atoms vary.
func (SemiNaive) materialize(b *Base, rules []Rule) error {
	byHead, strata, err := b.checkRules(rules)
	if err != nil {
		return err
	}
	for _, stratum := range strata {
		in := make(map[string]bool, len(stratum))
		for _, rel := range stratum {
			in[rel] = true
		}
		mark := marks(b, stratum)
		for _, rel := range stratum {
			for _, r := range byHead[rel] {
				if _, err := b.applyRule(r); err != nil {
					return err
				}
			}
		}
		delta := since(b, stratum, mark)
		for len(delta) > 0 {
			installDeltas(b, stratum, delta)
			mark = marks(b, stratum)
			for _, rel := range stratum {
				for _, r := range byHead[rel] {
					for i, lit := range r.Body.Literals {
						if lit.Pos == nil || !in[lit.Pos.Relation] || len(delta[lit.Pos.Relation]) == 0 {
							continue
						}
						if _, err := b.applyRule(readingDelta(r, i)); err != nil {
							return err
						}
					}
				}
			}
			delta = since(b, stratum, mark)
		}
		dropDeltas(b, stratum)
	}
	return nil
}

// marks records how many tuples each relation of a stratum holds, so since can tell what a round added.
func marks(b *Base, stratum []string) map[string]int {
	m := make(map[string]int, len(stratum))
	for _, rel := range stratum {
		m[rel] = len(b.idb[rel])
	}
	return m
}

// since returns, per relation, the tuples added after mark, leaving out relations that gained none.
// A derived relation only ever grows by appending, so they are the tail of its slice.
func since(b *Base, stratum []string, mark map[string]int) map[string][]idbTuple {
	out := map[string][]idbTuple{}
	for _, rel := range stratum {
		if tuples := b.idb[rel]; len(tuples) > mark[rel] {
			out[rel] = tuples[mark[rel]:]
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
	lits[i] = Literal{Pos: &a}
	return Rule{Head: r.Head, Body: Body{Literals: lits}, Hops: r.Hops, HeadTypes: r.HeadTypes}
}
