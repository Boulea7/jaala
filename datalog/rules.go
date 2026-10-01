package datalog

import (
	"fmt"
	"github.com/panyam/jaala/ns"
	"sort"
	"strings"
)

// idbTuple is one derived fact of a rule-defined (IDB) relation: positional values plus the
// provenance of the base facts that derived it, so a derived answer stays as verifiable as a
// looked-up one.
type idbTuple struct {
	vals  []ns.Value
	cites []string
}

// materialize evaluates the query's user-defined rules into b.idb by stratified fixpoint. Rules
// define derived (IDB) relations; a rule derives its head for every binding of its body, and
// recursion (a rule whose body reads its own head, directly or transitively) runs to fixpoint —
// which terminates because the fact base is finite and no rule invents new values (no function
// symbols). Negation is stratified: the program is rejected up front if any relation depends on the
// negation of a relation in its own recursive cycle, which is exactly what makes a `not` safe (the
// negated relation is fully derived before the stratum that reads it runs).
//
// The pipeline is: index rules by head + validate (no head redefines a fact/built-in relation,
// consistent arity, range-restricted heads, known body relations) -> stratify (assign each relation
// a stratum; reject recursion through negation) -> per stratum, run a naive fixpoint until no new
// tuple appears.
//
// Worked example — a transitive closure over a derived edge:
//
//	link(?a,?b) :- component-on-net(?a,?n), component-on-net(?b,?n), ?a != ?b   // reads only EDB
//	conn(?a,?b) :- link(?a,?b)
//	conn(?a,?c) :- conn(?a,?b), link(?b,?c)                                     // recursive (positive)
//
// stratify puts both link and conn in stratum 0 (conn's self-recursion is positive, so it may share
// link's stratum). The fixpoint first derives every link pair, then re-runs the two conn rules until
// no new conn tuple appears — which is where a chain U1-N1-U2-N2-U3 closes up so conn(U1,U3) exists
// even though U1 and U3 share no net directly. See stratify for the strata assignment.
func (b *Base) materialize(rules []Rule) error {
	byHead, strata, err := b.checkRules(rules)
	if err != nil {
		return err
	}
	for _, stratum := range strata {
		for { // naive fixpoint: re-derive every rule in the stratum until nothing new appears
			changed := false
			for _, rel := range stratum {
				for _, r := range byHead[rel] {
					added, err := b.applyRule(r)
					if err != nil {
						return err
					}
					changed = changed || added
				}
			}
			if !changed {
				break
			}
		}
	}
	return nil
}

// checkRules validates a rule set and returns what evaluating it needs: the rules grouped by head,
// and the strata to derive them in. It populates b.idbArity as it goes, since a rule's own head is
// what gives a derived relation its arity.
//
// It is separated from materialize because Validate runs it WITHOUT a design (agni issue 540). Every
// check here reads the rules and the relation vocabulary, never a row, so a broken rule set can be
// rejected where the rule is built rather than where it first runs.
func (b *Base) checkRules(rules []Rule) (map[string][]Rule, [][]string, error) {
	byHead := map[string][]Rule{}
	for _, r := range rules {
		rel := r.Head.Relation
		if _, ok := b.schemaOf(rel); ok {
			return nil, nil, fmt.Errorf("query: rule head %q redefines a fact relation", rel)
		}
		if _, ok := b.reg.Predicate(rel); ok {
			return nil, nil, fmt.Errorf("query: rule head %q redefines a built-in relation", rel)
		}
		ar := len(r.Head.Args)
		if prev, ok := b.idbArity[rel]; ok && prev != ar {
			return nil, nil, fmt.Errorf("query: rule %q defined with %d and %d args (arity must be consistent)", rel, prev, ar)
		}
		b.idbArity[rel] = ar
		byHead[rel] = append(byHead[rel], r)
	}
	for _, r := range rules {
		if err := b.validateRule(r); err != nil {
			return nil, nil, err
		}
	}
	strata, err := stratify(rules, b.idbArity)
	if err != nil {
		return nil, nil, err
	}
	return byHead, strata, nil
}

// validateRule checks a single rule's well-formedness independent of evaluation order: every body
// relation is known, and every head variable is bound by a positive body literal (range
// restriction, so a derived tuple never carries an unbound value). A variable that appears only
// under negation stays existential — the same lenient reading the goal uses — so it is not required
// in the head and is not checked here.
//
// Example:
//
//	bad(?x,?y) :- component-on-net(?x,?n)                 // rejected: ?y is in the head but no
//	                                                      //   positive body literal binds it
//	ok(?x)     :- component-on-net(?x,?n), not linked(?x) // accepted: ?x is bound positively; that
//	                                                      //   it also appears under `not` is irrelevant
func (b *Base) validateRule(r Rule) error {
	for _, lit := range r.Body.Literals {
		var rel string
		switch {
		case lit.Pos != nil:
			rel = lit.Pos.Relation
		case lit.Neg != nil:
			rel = lit.Neg.Relation
		default:
			continue // a comparison has no relation
		}
		// Only when there IS a vocabulary to be unknown in. An empty vocabulary cannot distinguish a
		// misspelled relation from one nobody installed, and answering "unknown" there would be a
		// statement about the query that the vocabulary has no standing to make (see Validate).
		if len(b.reg.BaseRelations()) > 0 && !b.knownRelation(rel) {
			return fmt.Errorf("query: rule %q reads %s", r.Head.Relation, b.reg.Unknown(rel))
		}
	}
	bound := map[Var]bool{}
	for _, vv := range positiveVars(r.Body) {
		bound[vv] = true
	}
	for _, arg := range r.Head.Args {
		if arg.Var != "" && arg.Var != "_" && !bound[arg.Var] {
			return fmt.Errorf("query: rule %q head variable ?%s is not bound by a positive body relation", r.Head.Relation, arg.Var)
		}
	}
	return nil
}

// knownRelation reports whether a relation name resolves to something the evaluator can read: an EDB
// fact relation, a built-in (reaches or a string filter), or a rule-defined IDB relation.
func (b *Base) knownRelation(rel string) bool {
	if _, ok := b.reg.Predicate(rel); ok {
		return true
	}
	if _, ok := b.schemaOf(rel); ok {
		return true
	}
	return b.isIDB(rel)
}

// applyRule solves one rule's body and adds a head tuple per solution. It reuses the goal's solver
// (positive backtracking join + post-solve negation filter), so a rule body has the full body
// expressiveness the goal has. Returns whether any new (deduplicated) tuple was added this pass.
func (b *Base) applyRule(r Rule) (bool, error) {
	pos, negs := splitNegations(r.Body.Literals)
	if err := b.validateNegations(r.Body, negs); err != nil {
		return false, err
	}
	added := false
	err := solve(pos, 0, newBinding(), b, func(bnd *binding) error {
		ok, err := passesNegations(bnd, negs, b)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		vals := make([]ns.Value, len(r.Head.Args))
		for j, arg := range r.Head.Args {
			val, ok := resolve(arg, bnd)
			if !ok {
				return nil // guarded by validateRule; belt-and-suspenders for a head var the body left unbound
			}
			vals[j] = val
		}
		if b.addTuple(r.Head.Relation, idbTuple{vals: vals, cites: dedupStrings(bnd.cites)}) {
			added = true
		}
		return nil
	})
	return added, err
}

// addTuple appends a derived tuple to its relation unless an equal-valued one is already present
// (set semantics — datalog facts have value identity). The first derivation's cites are kept, so
// provenance is deterministic under the fixpoint's fixed rule and tuple order.
//
// The membership test is a bucket lookup rather than a scan of the relation. It used to
// be linear, so deriving n tuples cost O(n^2) before any join work: a transitive closure over a
// 4,000-component design spent 28 seconds here. valsEqual still decides within the bucket, so set
// semantics and the first-wins provenance rule are unchanged — only the number of comparisons is.
func (b *Base) addTuple(rel string, t idbTuple) bool {
	tuples := b.idb[rel]
	x := b.idbIndexFor(rel, fullMask(len(t.vals)))
	x.sync(tuples, fullMask(len(t.vals)))
	for _, k := range tupleKeys(t.vals) {
		for _, i := range x.buckets[k] {
			b.countWork()
			if valsEqual(tuples[i].vals, t.vals) {
				return false
			}
		}
	}
	b.idb[rel] = append(tuples, t)
	return true
}

// idbIndexFor returns this query's index of a derived relation at one binding pattern, creating it
// on first use. Per-query because a derived relation only exists for one query; see Base.idbIdx.
func (b *Base) idbIndexFor(rel string, mask patternMask) *idbIndex {
	if b.idbIdx == nil {
		b.idbIdx = map[idxKey]*idbIndex{}
	}
	k := idxKey{rel: rel, mask: mask}
	x, ok := b.idbIdx[k]
	if !ok {
		x = &idbIndex{}
		b.idbIdx[k] = x
	}
	return x
}

func valsEqual(a, b []ns.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valueEq(a[i], b[i]) {
			return false
		}
	}
	return true
}

// stratify assigns each IDB relation a stratum such that a relation's stratum is >= any relation it
// reads positively and strictly > any it reads under negation, then returns the relation names
// grouped by stratum in ascending order. It rejects a program that reads a relation under negation
// from inside that relation's own recursive cycle (unstratifiable negation) — the standard condition
// that keeps `not` well-defined.
//
// Algorithm: iterative relaxation over the predicate-dependency edges (a longest-path / Bellman-Ford
// shape), NOT strongly-connected-components. Each round raises a relation's stratum to satisfy its
// edges — `>=` for a positive edge, strictly `>` for a negative one. A positive cycle is harmless: it
// only equalizes (max, no increment), so there is no need to condense SCCs first. Only a negative
// edge increments, so a negative edge inside a cycle would force the stratum to climb without bound;
// that is caught by failing to converge within |relations| rounds, and the non-convergence IS the
// recursion-through-negation signal. Deterministic and linear in the rules.
//
// Example:
//
//	linked(?a)   :- component-on-net(?a,?n), ...              // reads only EDB      -> stratum 0
//	isolated(?r) :- component-on-net(?r,?n), not linked(?r)   // negates linked      -> stratum 1
//
// isolated's negative edge to linked forces stratum(isolated) > stratum(linked), so linked is fully
// derived before isolated is computed — the guarantee that makes the `not` sound. Contrast the
// unstratifiable `p :- ..., not q` with `q :- ..., not p`: each negative edge forces the other
// strictly higher every round, relaxation never settles, and the program is rejected.
func stratify(rules []Rule, arity map[string]int) ([][]string, error) {
	var edges []depEdge
	for _, r := range rules {
		head := r.Head.Relation
		for _, lit := range r.Body.Literals {
			atom, neg := lit.Pos, false
			if lit.Neg != nil {
				atom, neg = lit.Neg, true
			}
			if atom == nil {
				continue // comparison
			}
			if _, isIDB := arity[atom.Relation]; isIDB {
				edges = append(edges, depEdge{from: head, to: atom.Relation, neg: neg})
			}
		}
	}
	stratum := map[string]int{}
	for rel := range arity {
		stratum[rel] = 0
	}
	n := len(arity)
	for round := 0; round <= n; round++ {
		changed := false
		for _, e := range edges {
			want := stratum[e.to]
			if e.neg {
				want++
			}
			if stratum[e.from] < want {
				stratum[e.from] = want
				changed = true
			}
		}
		if !changed {
			break
		}
		if round == n {
			return nil, fmt.Errorf("query: rules are not stratifiable (recursion through negation: %s)", strings.Join(negativeCycle(edges), ", "))
		}
	}
	byStratum := map[int][]string{}
	maxS := 0
	for rel, s := range stratum {
		byStratum[s] = append(byStratum[s], rel)
		if s > maxS {
			maxS = s
		}
	}
	var out [][]string
	for s := 0; s <= maxS; s++ {
		if group := byStratum[s]; len(group) > 0 {
			sort.Strings(group) // deterministic order within a stratum
			out = append(out, group)
		}
	}
	return out, nil
}

// depEdge is one rule dependency: from's rule reads to, under negation when neg.
type depEdge struct {
	from, to string
	neg      bool
}

// negativeCycle names the relations whose rules read, under negation, a relation that depends back on
// them: the relations a recursion-through-negation error is about. Private names are shown as written.
func negativeCycle(edges []depEdge) []string {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.from] = append(adj[e.from], e.to)
	}
	reaches := func(from, to string) bool {
		seen := map[string]bool{}
		stack := []string{from}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == to {
				return true
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, adj[n]...)
		}
		return false
	}
	var out []string
	for _, e := range edges {
		if e.neg && reaches(e.to, e.from) {
			out = append(out, e.from, e.to)
		}
	}
	return displayNames(out)
}
