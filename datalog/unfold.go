package datalog

import "strconv"

// unfold inlines calls to single-rule, non-recursive derived relations, replacing each call with the
// body of the relation's one rule. It is SemiNaive's, run before plan (see SemiNaive), and it exists
// for what the planner can do afterwards: a bound argument at the call reaches the literals inside, so
// `go.covers(?t, "X")` over `covers(?t, ?f) :- test(?t), graph.reach(?t, ?f, ...)` becomes one
// walk back from "X", where materializing covers walked out from every test.
//
// A call is inlined only when doing so cannot change an answer:
//
//   - The relation is not recursive, so inlining terminates.
//   - It has one rule. Several rules are a disjunction, which a goal cannot hold (see #28).
//   - Its head is distinct variables, or constants the call matches with constants or "_". A head
//     constant meeting a caller variable would need "=" to bind, and it does not.
//   - The call is positive. "not p(...)" negates the whole body, not each literal of it.
//   - In the goal, no aggregate counts bindings: count, sum and list without distinct. A derived
//     relation is a set, so has_tp(?n) yields each net once, but its inlined body yields a net once per
//     test point on it, and `=> count(?n)` would count those. A rule body is always safe, since its
//     head is a set, and min, max and distinct aggregates never count duplicates.
//
// A relation the goal no longer reaches once its calls are inlined loses its rules, so it is not
// materialized; a relation still called elsewhere, or under negation, keeps them.
func unfold(b *Base, q Query) Query {
	byHead := map[string][]Rule{}
	for _, r := range q.Rules {
		byHead[r.Head.Relation] = append(byHead[r.Head.Relation], r)
	}
	u := &unfolder{byHead: byHead, recursive: recursiveRelations(q.Rules)}
	out := q
	if len(out.Select) == 0 {
		out.Select = defaultSelect(q.Goal) // the answer's columns are the written goal's, not the inlined one's
	}
	out.Rules = make([]Rule, 0, len(q.Rules))
	for _, r := range q.Rules {
		r.Body = u.body(r.Body)
		out.Rules = append(out.Rules, r)
	}
	if !countsBindings(out.Select, q.Having) {
		out.Goal = u.body(q.Goal)
	}
	return dropUnreached(q, out)
}

type unfolder struct {
	byHead    map[string][]Rule
	recursive map[string]bool
	fresh     int
}

// body inlines every inlinable call in a body, including calls inside what it inlines.
func (u *unfolder) body(b Body) Body {
	var out []Literal
	queue := append([]Literal(nil), b.Literals...)
	for len(queue) > 0 {
		lit := queue[0]
		queue = queue[1:]
		if lit.Pos != nil {
			if lits, ok := u.inline(*lit.Pos); ok {
				queue = append(lits, queue...)
				continue
			}
		}
		out = append(out, lit)
	}
	return Body{Literals: out}
}

// inline returns the body that replaces a call, or false when the call must stay.
func (u *unfolder) inline(call Atom) ([]Literal, bool) {
	rules := u.byHead[call.Relation]
	if len(rules) != 1 || u.recursive[call.Relation] {
		return nil, false
	}
	r := rules[0]
	if len(r.Head.Args) != len(call.Args) {
		return nil, false
	}
	u.fresh++
	sub := map[Var]Term{}
	for i, h := range r.Head.Args {
		c := call.Args[i]
		switch {
		case h.Const != nil:
			if !(c.Var == "_" || (c.Const != nil && valueEq(*h.Const, *c.Const))) {
				return nil, false
			}
		case h.Var == "" || h.Var == "_":
			return nil, false
		default:
			if _, repeated := sub[h.Var]; repeated {
				return nil, false
			}
			if c.Var == "_" {
				c = Term{Var: u.freshVar(h.Var)}
			}
			sub[h.Var] = c
		}
	}
	rename := func(t Term) Term {
		if t.Var == "" || t.Var == "_" {
			return t
		}
		if s, ok := sub[t.Var]; ok {
			return s
		}
		s := Term{Var: u.freshVar(t.Var)}
		sub[t.Var] = s
		return s
	}
	atom := func(a Atom) *Atom {
		out := Atom{Relation: a.Relation, Args: make([]Term, len(a.Args))}
		for i, t := range a.Args {
			out.Args[i] = rename(t)
		}
		return &out
	}
	var lits []Literal
	for _, l := range r.Body.Literals {
		switch {
		case l.Pos != nil:
			lits = append(lits, Literal{Pos: atom(*l.Pos)})
		case l.Neg != nil:
			lits = append(lits, Literal{Neg: atom(*l.Neg)})
		default:
			c := *l.Compare
			c.Left, c.Right = rename(c.Left), rename(c.Right)
			lits = append(lits, Literal{Compare: &c})
		}
	}
	return lits, true
}

// freshVar is a variable no query can spell, distinct per inlined call, so an inlined body's own
// variables never meet the caller's.
func (u *unfolder) freshVar(name Var) Var {
	return Var("\x00" + strconv.Itoa(u.fresh) + "." + string(name))
}

// recursiveRelations is every rule head that depends on itself, through positive or negated reads.
func recursiveRelations(rules []Rule) map[string]bool {
	reads := map[string][]string{}
	for _, r := range rules {
		for _, a := range ruleAtoms(r.Body) {
			reads[r.Head.Relation] = append(reads[r.Head.Relation], a.Relation)
		}
	}
	out := map[string]bool{}
	for head := range reads {
		seen := map[string]bool{}
		stack := append([]string(nil), reads[head]...)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == head {
				out[head] = true
				break
			}
			if !seen[n] {
				seen[n] = true
				stack = append(stack, reads[n]...)
			}
		}
	}
	return out
}

// countsBindings reports whether an aggregate in the answer counts bindings rather than values: count,
// sum or list without distinct. Inlining into such a goal could change its answer.
func countsBindings(sel []Term, having []Compare) bool {
	aggs := make([]*Aggregate, 0, len(sel)+len(having))
	for _, t := range sel {
		aggs = append(aggs, t.Agg)
	}
	for _, h := range having {
		aggs = append(aggs, h.Left.Agg)
	}
	for _, a := range aggs {
		if a != nil && !a.Distinct && (a.Func == "count" || a.Func == "sum" || a.Func == "list") {
			return true
		}
	}
	return false
}

// dropUnreached removes the rules that the goal reached before unfolding and no longer does: a relation
// whose every call was inlined, and anything only it read. Rules the goal never reached, such as a
// query's own unused rule, are left as they were.
func dropUnreached(before, after Query) Query {
	was, is := reached(before), reached(after)
	kept := after.Rules[:0:0]
	for _, r := range after.Rules {
		if is[r.Head.Relation] || !was[r.Head.Relation] {
			kept = append(kept, r)
		}
	}
	after.Rules = kept
	return after
}

// reached is every relation the goal reads, directly or through rules.
func reached(q Query) map[string]bool {
	reads := map[string][]string{}
	for _, r := range q.Rules {
		for _, a := range ruleAtoms(r.Body) {
			reads[r.Head.Relation] = append(reads[r.Head.Relation], a.Relation)
		}
	}
	out := map[string]bool{}
	var stack []string
	for _, a := range ruleAtoms(q.Goal) {
		stack = append(stack, a.Relation)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !out[n] {
			out[n] = true
			stack = append(stack, reads[n]...)
		}
	}
	return out
}
