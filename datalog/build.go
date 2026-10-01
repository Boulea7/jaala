package datalog

import (
	"fmt"
	"github.com/panyam/jaala/ns"
	"sort"
)

// This file is the programmatic constructor for a Query — the typed alternative to Parse. A
// hand-authored rule is clearest as a string (MustParse); a GENERATED rule (one a host compiles from
// its own configuration) is safer as an AST: no separator or quoting to get wrong, and a malformed program is a
// compile error, not a parse panic at init. Both produce the same Query the evaluator runs.

// V builds a variable term (?name in the text syntax).
func V(name string) Term { return Term{Var: Var(name)} }

// Str builds a string-constant term.
func Str(s string) Term { return Term{Const: &ns.Value{S: s}} }

// Num builds a numeric-constant term.
func Num(f float64) Term { return Term{Const: &ns.Value{S: ftoa(f), Num: &f}} }

// Rel builds an atom Relation(args...) — an EDB relation, a built-in, or an IDB relation a Def
// introduces.
func Rel(name string, args ...Term) Atom { return Atom{Relation: name, Args: args} }

// Pos wraps an atom as a positive body literal.
func Pos(a Atom) Literal { return Literal{Pos: &a} }

// Neg wraps an atom as a negated body literal (stratified negation).
func Neg(a Atom) Literal { return Literal{Neg: &a} }

// Cmp builds a comparison literal, Op in {<, <=, =, !=, >, >=}.
func Cmp(l Term, op string, r Term) Literal {
	return Literal{Compare: &Compare{Left: l, Op: op, Right: r}}
}

// Def builds an IDB rule Head :- body. Disjunction is several Defs sharing a Head relation.
func Def(head Atom, body ...Literal) Rule { return Rule{Head: head, Body: Body{Literals: body}} }

// Build assembles a Query from its IDB rules, the goal literals to solve, and the projected answer
// terms (the => columns). Passing no rules is the common single-goal case.
func Build(rules []Rule, goal []Literal, sel ...Term) Query {
	return Query{Rules: rules, Goal: Body{Literals: goal}, Select: sel}
}

// MustParse parses a query string and panics on error — for hand-authored rules constructed at
// package init, where a malformed built-in query is a programming error surfaced at startup (the
// same contract RuleFromQuery had when it parsed internally).
func MustParse(s string) Query {
	q, err := Parse(s)
	if err != nil {
		panic(fmt.Sprintf("datalog.MustParse(%q): %v", s, err))
	}
	return q
}

// Reads returns the base relations a query references, the ones reg holds as base relations, sorted
// and deduped. Rule-defined relations and predicates are excluded; only base relations count as
// reads. A host uses it to declare which facts a query-backed check depends on.
//
// It counts reads inside the derived modules the query links in, since a check built on
// net.has_test_point depends on whatever net.has_test_point reads. When the query does not link, it
// reports the query's own reads, and Validate says why.
func Reads(q Query, reg *ns.Vocabulary) []string {
	if linked, err := Link(q, reg); err == nil {
		q = linked
	}
	seen := map[string]bool{}
	visit := func(b Body) {
		for _, l := range b.Literals {
			for _, a := range []*Atom{l.Pos, l.Neg} {
				if a == nil {
					continue
				}
				if _, ok := reg.Schema(a.Relation); ok {
					seen[a.Relation] = true
				}
			}
		}
	}
	for _, r := range q.Rules {
		visit(r.Body)
	}
	visit(q.Goal)
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// GeneratorFirstRules reports the rules that OPEN their body with a value-producing generator whose
// own input argument is unbound, naming each offender by its head relation. Empty means no rule starts
// by enumerating the whole design. It runs on the linked query, so a module rule the query reaches is
// checked as well as the query's own.
//
// This is the shape that makes a generated rule non-terminating rather than merely slow. The evaluator
// is a naive backtracking join running literals left to right, and `reaches` is the one built-in that
// PRODUCES values instead of filtering: given a bound start net it walks that net's series
// neighborhood, but given an unbound one it walks from EVERY net on the board. In first position
// nothing can have bound it, so the walk is unconditional. A shipped profile rule opened with
// `reaches(?n, ?rn, ?h)` and took `agni check` from 13s to not finishing at all on a real design
// . The fix was to lead with the guard the consuming rule already conjoined, which binds the
// start net before the walk begins.
//
// A generator whose input is a CONSTANT is fine (`reaches("VBUS", ?n, ?h)` is one walk), so only an
// unbound first argument is reported.
//
// SCOPE, stated because it is narrower than it looks. This catches the catastrophic case, not every
// bad join order. `pulled` opened with `component-on-net(?pu, ?n)`, both variables unbound, which is a
// full EDB scan re-entered per survivor: 21s rather than forever, and NOT reported here. A general
// answer is a cost-based join planner, not a lint. Note also what does NOT work as a guard,
// since it is the obvious first idea: requiring the first literal to share a variable with the head
// catches neither offender, because both `reaches(?n, ...)` and `component-on-net(?pu, ?n)` do mention
// the head variable. Mentioning it is not binding it.
func GeneratorFirstRules(q Query, reg *ns.Vocabulary) []string {
	if linked, err := Link(q, reg); err == nil {
		q = linked
	}
	var out []string
	for _, r := range q.Rules {
		for _, l := range r.Body.Literals {
			if l.Pos == nil {
				continue // a comparison or negation cannot enumerate, so it is not the opening scan
			}
			bi, ok := reg.Predicate(l.Pos.Relation)
			if ok && bi.IsGenerator() && len(l.Pos.Args) > 0 && l.Pos.Args[0].Const == nil {
				out = append(out, r.Head.Relation)
			}
			break // only the FIRST positive literal opens the scan
		}
	}
	return displayNames(out)
}

// NonInjectiveRules reports rules whose body puts two DIFFERENT variables in the SAME argument
// position of the SAME relation without separating them, naming each offender by its head relation.
// Empty means every such pair is separated.
//
// Datalog matches by homomorphism, so two body variables may bind the same node. Circuit intent is
// almost always the opposite: a divider's two resistors must be two parts, a pull-up's rail must not
// be the net it pulls up. The language cannot say that, so an author writes a disequality by hand
// and forgetting one is neither a parse error nor a crash. It is a silently wrong verdict.
//
// The shipped case that shows the cost: the interface-presence rule derives "in use" from two
// matching signals, `has_signal(?x), has_signal(?y), ?x != ?y`. Drop that comparison and one
// matching signal satisfies "two distinct signals are present", so an interface reports itself in
// use on half the evidence and every completeness check scoped to it inherits the mistake, with
// nothing in the output saying so.
//
// SCOPE, stated because it is narrower than it looks, and deliberately so.
//
// It reports SAME-relation, SAME-position pairs only. That is the shape both shipped disequalities
// guard, and it is the one where two variables are interchangeable by construction: the relation
// says what the position means, so two variables there range over the same set. Comparing variables
// ACROSS positions or across different relations would be guesswork about sorts the query language
// does not carry.
//
// It is therefore silent on a rule like
//
//	clamped(?n) :- reaches(?n, ?rn, ?h), ?h <= 2, component-on-net(?t, ?rn), component.class(?t, "tvs")
//
// where ?n and ?rn are both nets and no disequality relates them. That is CORRECT rather than a
// miss: `reaches` is reflexive (from == to at distance zero), so a clamp on the net itself is a
// genuine answer and separating the two would break the rule. Two positions of one atom are not the
// interchangeable shape; two atoms of one relation are.
//
// Like GeneratorFirstRules it examines rule bodies, not the goal, because a goal is written per
// query by someone reading its result while a rule is authored once and consumed silently.
func NonInjectiveRules(q Query) []string {
	var out []string
	for _, r := range q.Rules {
		if nonInjectiveBody(r.Body) {
			out = append(out, r.Head.Relation)
		}
	}
	sort.Strings(out)
	return out
}

// nonInjectiveBody reports whether the body holds two PARALLEL atoms of one relation — identical
// except for distinct variables at a single position — that no disequality separates.
//
// The "identical except at one position" test is what keeps this quiet on rules that distinguish
// their atoms some other way. The shipped termination rule reads
//
//	terminated(?h) :- component-on-net(?r,?h), suffix(?h,"_CANH"), reaches(?h,?l), suffix(?l,"_CANL")
//
// where suffix appears twice with ?h and ?l at position 0 and nothing separating them. Flagging it
// would be wrong: the two atoms carry DIFFERENT constants at position 1, so the author has already
// said these are different questions, and demanding a disequality would be noise on a correct rule.
// Only when every other argument matches are the two atoms genuinely interchangeable, which is
// exactly when homomorphic matching can collapse them and mean something the author did not write.
func nonInjectiveBody(b Body) bool {
	separated := map[[2]Var]bool{}
	for _, l := range b.Literals {
		c := l.Compare
		if c == nil || c.Op != "!=" || c.Left.Var == "" || c.Right.Var == "" {
			continue
		}
		separated[[2]Var{c.Left.Var, c.Right.Var}] = true
		separated[[2]Var{c.Right.Var, c.Left.Var}] = true
	}
	var atoms []*Atom
	for _, l := range b.Literals {
		if l.Pos != nil { // negation cannot bind, and a comparison has no argument positions
			atoms = append(atoms, l.Pos)
		}
	}
	for i, a := range atoms {
		for _, c := range atoms[i+1:] {
			if v, w, ok := parallelPair(a, c); ok && !separated[[2]Var{v, w}] {
				return true
			}
		}
	}
	return false
}

// parallelPair reports the one position at which two atoms of the same relation differ, when they
// differ ONLY there and by two distinct variables. ok is false for atoms of different relations,
// different arity, or differing in any other way.
func parallelPair(a, c *Atom) (v, w Var, ok bool) {
	if a.Relation != c.Relation || len(a.Args) != len(c.Args) {
		return "", "", false
	}
	found := -1
	for i := range a.Args {
		if sameTerm(a.Args[i], c.Args[i]) {
			continue
		}
		if found >= 0 {
			return "", "", false // differs in more than one position: not a parallel pair
		}
		found = i
	}
	if found < 0 {
		return "", "", false // identical atoms: a repeated literal, not two nodes
	}
	av, cv := a.Args[found].Var, c.Args[found].Var
	if av == "" || av == "_" || cv == "" || cv == "_" {
		return "", "", false // a constant or wildcard on either side is not a colliding variable
	}
	return av, cv, true
}

// sameTerm reports whether two argument terms are the same variable or the same constant.
func sameTerm(a, b Term) bool {
	if a.Var != "" || b.Var != "" {
		return a.Var == b.Var
	}
	if a.Const == nil || b.Const == nil {
		return a.Const == b.Const
	}
	return valueEq(*a.Const, *b.Const)
}
