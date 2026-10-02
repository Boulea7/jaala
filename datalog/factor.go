package datalog

import (
	"strconv"
	"strings"
)

// factor is magic sets' rewrite for right-linear recursion, applied to a goal call whose bound
// arguments are all constants (#35). Asked reach("v0", ?x) of
//
//	reach(?a, ?b) :- edge(?a, ?b);
//	reach(?a, ?c) :- edge(?a, ?b), reach(?b, ?c);
//
// plain magic sets derive reach_bf(?b, ?c) for every ?b that v0 reaches, each with its full answer set,
// which is quadratic on a chain. But the recursive rule hands its answer ?c back unchanged, so every
// answer on the way down is an answer for v0, and only the set of nodes the recursion visits matters:
//
//	from("v0").
//	from(?b) :- from(?a), edge(?a, ?b);
//	answer("v0", ?b) :- from(?a), edge(?a, ?b);
//
// and the goal reads answer("v0", ?x), which is linear in the edges v0 reaches.
//
// A relation qualifies when it is eligible for demand and each of its rules is a base rule, whose body
// does not reach the relation, or a right-linear one: its body calls the relation exactly once, nothing
// else in the body reaches it, the call's free arguments are the head's free arguments, those are
// distinct variables appearing nowhere else in the rule, and the rest of the body, run from the head's
// bound arguments, binds the call's bound ones.
//
// Only a goal call with constants is factored: it has one start, so one set serves it. A call bound by
// a variable has a start per value, each with its own answers, and goes through plain magic sets. A
// witnessed Eval is not factored: its witnesses mirror the rules as written, and the factoring folds
// away the levels of the recursion they would show. The set is an ordinary derived relation, so an
// answer's citations are the facts of one path from the start.
func (m *magician) factor(a Atom, bound map[Var]bool) (Atom, bool) {
	if _, derived := m.byHead[a.Relation]; !derived || m.b.witnessing() || m.aggregates(a.Relation) {
		return a, false
	}
	adorn, demanded := adornment(a, bound)
	if !strings.Contains(adorn, "b") {
		return a, false
	}
	for _, t := range demanded {
		if t.Const == nil {
			return a, false
		}
	}
	calls := make([]int, len(m.byHead[a.Relation]))
	for i, r := range m.byHead[a.Relation] {
		call, ok := m.rightLinear(r, adorn)
		if !ok {
			return a, false
		}
		calls[i] = call
	}
	m.factored++
	from := a.Relation + fromSep + strconv.Itoa(m.factored)
	answer := a.Relation + "\x00answer" + strconv.Itoa(m.factored)
	m.rules = append(m.rules, Rule{Head: Atom{Relation: from, Args: demanded}})
	for i, r := range m.byHead[a.Relation] {
		guard := &Atom{Relation: from, Args: boundTerms(r.Head, adorn)}
		if calls[i] < 0 {
			head := Atom{Relation: answer, Args: make([]Term, len(a.Args))}
			for j, t := range r.Head.Args {
				if adorn[j] == 'b' {
					t = a.Args[j]
				}
				head.Args[j] = t
			}
			m.rules = append(m.rules, Rule{Head: head, Body: m.guarded(guard, r.Body.Literals), Hops: r.Hops, HeadTypes: r.HeadTypes, text: r.text})
			continue
		}
		lits := r.Body.Literals
		rest := append(append([]Literal{}, lits[:calls[i]]...), lits[calls[i]+1:]...)
		head := Atom{Relation: from, Args: boundTerms(*lits[calls[i]].Pos, adorn)}
		m.rules = append(m.rules, Rule{Head: head, Body: m.guarded(guard, rest), text: r.text})
	}
	return Atom{Relation: answer, Args: a.Args}, true
}

// fromSep marks a factored reachable set's name. Like a magic relation, the set guards the rules that
// read it, so planning keeps it first (see plan).
const fromSep = "\x00from"

// rightLinear reports whether a rule of rel qualifies for factoring under adorn (see factor), with the
// index of its recursive call, or -1 for a base rule.
func (m *magician) rightLinear(r Rule, adorn string) (int, bool) {
	rel := r.Head.Relation
	call := -1
	for i, lit := range r.Body.Literals {
		switch {
		case lit.Neg != nil:
			return 0, false
		case lit.Pos == nil:
		case lit.Pos.Relation == rel:
			if call >= 0 {
				return 0, false
			}
			call = i
		case m.reaches(lit.Pos.Relation, rel):
			return 0, false
		}
	}
	if call < 0 {
		return -1, true
	}
	free := map[Var]bool{}
	c := r.Body.Literals[call].Pos
	for i, t := range r.Head.Args {
		if adorn[i] == 'b' {
			continue
		}
		if t.Var == "" || t.Var == "_" || free[t.Var] || c.Args[i].Var != t.Var {
			return 0, false
		}
		free[t.Var] = true
	}
	mentions := append(append([]Term{}, boundTerms(r.Head, adorn)...), boundTerms(*c, adorn)...)
	for i, lit := range r.Body.Literals {
		if i != call {
			mentions = append(mentions, literalTerms(lit)...)
		}
	}
	for _, t := range mentions {
		if free[t.Var] {
			return 0, false
		}
	}
	guard := Atom{Relation: "\x00guard", Args: boundTerms(r.Head, adorn)}
	body := Body{Literals: []Literal{{Pos: &guard}}}
	for i, lit := range r.Body.Literals {
		if i != call {
			body.Literals = append(body.Literals, lit)
		}
	}
	if checkModes(m.b, "", body) != nil {
		return 0, false
	}
	bound := map[Var]bool{}
	for _, lit := range body.Literals {
		if lit.Pos != nil && !isCheck(m.b, lit) {
			bindAll(lit.Pos, bound)
		}
	}
	for _, t := range boundTerms(*c, adorn) {
		if t.Var != "" && !bound[t.Var] {
			return 0, false
		}
	}
	return call, true
}

// guarded is a factored rule's body: the set's guard, then lits ordered from what it binds, rewritten
// for demand as an adorned rule's are.
func (m *magician) guarded(guard *Atom, lits []Literal) Body {
	entry := map[Var]bool{}
	bindAll(guard, entry)
	return Body{Literals: m.body(guard, planBody(m.b, Body{Literals: lits}, entry).Literals, entry, true)}
}

// reaches reports whether rel's rules read target, directly or through other rules.
func (m *magician) reaches(rel, target string) bool {
	seen := map[string]bool{}
	stack := []string{rel}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == target {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, r := range m.byHead[n] {
			for _, a := range ruleAtoms(r.Body) {
				stack = append(stack, a.Relation)
			}
		}
	}
	return false
}

// boundTerms is a's arguments at adorn's bound positions.
func boundTerms(a Atom, adorn string) []Term {
	var out []Term
	for i, t := range a.Args {
		if adorn[i] == 'b' {
			out = append(out, t)
		}
	}
	return out
}
