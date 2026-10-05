package datalog

import (
	"fmt"
	"strings"

	"github.com/panyam/jaala/ns"
)

// checkModes refuses a body in which some generator can never be called in a mode it accepts,
// whatever order its literals run in. It computes the variables the body can bind, starting from
// nothing and adding each atom's variables once that atom could run: a relation can always run, a
// generator once one of its modes is satisfied, and a filter or comparison binds nothing. A
// generator left over when nothing more can bind was never callable.
//
// The check does not depend on clause order, so every evaluator reports the same error for the same
// body, whether it runs the body as written (Naive) or plans it (SemiNaive), and Validate reports it
// without evaluating. where names the body for the message: a rule's head, or "the query".
func checkModes(b *Base, where string, body Body) error {
	bound := map[Var]bool{}
	done := make([]bool, len(body.Literals))
	for changed := true; changed; {
		changed = false
		for i, lit := range body.Literals {
			if done[i] || lit.Pos == nil {
				continue
			}
			bi, isPred := b.reg.Predicate(lit.Pos.Relation)
			switch {
			case isPred && !bi.IsGenerator():
				continue // a filter binds nothing
			case isPred && !bi.Satisfied(boundFlags(lit.Pos, bound)):
				continue // not callable yet; maybe once more is bound
			}
			done[i], changed = true, true
			bindAll(lit.Pos, bound)
		}
	}
	for i, lit := range body.Literals {
		a := lit.Pos
		if a == nil {
			a = lit.Neg // a negated generator runs over what the positive body bound
		} else if done[i] {
			continue
		}
		if a == nil {
			continue
		}
		if bi, ok := b.reg.Predicate(a.Relation); ok && bi.IsGenerator() && !bi.Satisfied(boundFlags(a, bound)) {
			return fmt.Errorf("query: %s calls %s, and nothing binds what %s needs first: it needs %s",
				where, a, a.Relation, modesText(bi, a))
		}
	}
	return nil
}

// boundFlags reports, per argument, whether it is a constant or a variable already bound.
func boundFlags(a *Atom, bound map[Var]bool) []bool {
	out := make([]bool, len(a.Args))
	for i, t := range a.Args {
		out[i] = t.Const != nil || (t.Var != "" && t.Var != "_" && bound[t.Var])
	}
	return out
}

func bindAll(a *Atom, bound map[Var]bool) {
	for _, t := range a.Args {
		if t.Var != "" && t.Var != "_" {
			bound[t.Var] = true
		}
	}
}

// modesText says what a generator's modes need, in the terms of one call: "?t bound, or ?f bound".
func modesText(bi ns.Builtin, a *Atom) string {
	var alts []string
	for _, m := range bi.Modes {
		var need []string
		for i, req := range m {
			if req && i < len(a.Args) {
				need = append(need, a.Args[i].String())
			}
		}
		alts = append(alts, strings.Join(need, " and ")+" bound")
	}
	return strings.Join(alts, ", or ")
}

// whereRule names a rule for a checkModes message.
func whereRule(r Rule) string { return fmt.Sprintf("rule %q", displayName(r.Head.Relation)) }

// plan reorders a query's rule bodies and goal so each literal runs once as much as possible is
// bound, which is SemiNaive's default (see SemiNaive.WrittenOrder). It changes no answer: a body is a
// conjunction, so its literals commute, and negated literals stay where solving already puts them,
// after the positive body.
//
// The query's columns are fixed first, from the goal as written, so reordering the goal does not
// reorder the answer.
func plan(b *Base, q Query) Query {
	out := q
	if len(out.Select) == 0 {
		out.Select = defaultSelect(q.Goal)
	}
	out.Rules = make([]Rule, len(q.Rules))
	for i, r := range q.Rules {
		r.Body = planRule(b, r.Body)
		out.Rules[i] = r
	}
	out.Goal = planBody(b, q.Goal, nil)
	return out
}

// planRule plans a rule body. A body the demand rewrite guarded keeps its guard first and is planned
// from what the guard binds: the guard holds only the demanded values, and ranked from nothing bound
// it would score as an unbound scan and fall behind a relation bound by constants, undoing the
// demand.
func planRule(b *Base, body Body) Body {
	lits := body.Literals
	if len(lits) == 0 || lits[0].Pos == nil || !isGuard(lits[0].Pos.Relation) {
		return planBody(b, body, nil)
	}
	entry := map[Var]bool{}
	bindAll(lits[0].Pos, entry)
	rest := planBody(b, Body{Literals: lits[1:]}, entry)
	return Body{Literals: append([]Literal{lits[0]}, rest.Literals...)}
}

// planGoal plans the goal for the demand rewrite. The relations set-bound variables range over (see
// bindSets) stay first, and the rest is planned from what they bind, as planRule plans a guarded body:
// ranked from nothing bound, they would fall behind a relation bound by constants, and a call would be
// demanded from that relation's values instead of the host's. Once the rewrite has seeded demand from
// them, plan orders the goal like any other, since the adorned relations hold only what was demanded.
func planGoal(b *Base, goal Body) Body {
	lits := goal.Literals
	n := 0
	for n < len(lits) && lits[n].Pos != nil && isBindSet(lits[n].Pos.Relation) {
		n++
	}
	if n == 0 {
		return planBody(b, goal, nil)
	}
	entry := map[Var]bool{}
	for _, l := range lits[:n] {
		bindAll(l.Pos, entry)
	}
	rest := planBody(b, Body{Literals: lits[n:]}, entry)
	return Body{Literals: append(append([]Literal(nil), lits[:n]...), rest.Literals...)}
}

// planBody orders a body greedily. At each step it takes the first check whose arguments are all
// bound, since checking early only discards: a comparison, a filter, or a relation whose arguments
// are all bound, which only asks whether a tuple exists. Failing that, the first generator one of
// whose Modes is satisfied (#36): a generator call is the host's own work, such as a graph walk, and
// one placed after a relation runs once per row of it, where a relation bound only by constants is
// a scan that costs a cheap comparison per row. Failing that, the relation with the most bound
// arguments, earliest written on a tie, since it fans out least and binds the most for what
// follows. A literal nothing can make runnable keeps its written
// place at the end, where solving reports it as it always has. bound is what is bound on entry, such
// as a rule head's demanded arguments (see magic); it is not changed.
func planBody(b *Base, body Body, entry map[Var]bool) Body {
	pos, negs := splitNegations(body.Literals)
	bound := map[Var]bool{}
	for v := range entry {
		bound[v] = true
	}
	var out []Literal
	for len(pos) > 0 {
		pick := -1
		for i, lit := range pos {
			if (isCheck(b, lit) || isProbe(b, lit)) && checkReady(lit, bound) {
				pick = i
				break
			}
		}
		if pick < 0 {
			for i, lit := range pos {
				if lit.Pos == nil {
					continue
				}
				if bi, ok := b.reg.Predicate(lit.Pos.Relation); ok && bi.IsGenerator() && bi.Satisfied(boundFlags(lit.Pos, bound)) {
					pick = i
					break
				}
			}
		}
		if pick < 0 {
			best := -1
			for i, lit := range pos {
				if isCheck(b, lit) {
					continue
				}
				flags := boundFlags(lit.Pos, bound)
				if bi, ok := b.reg.Predicate(lit.Pos.Relation); ok && !bi.Satisfied(flags) {
					continue
				}
				if n := count(flags); n > best {
					pick, best = i, n
				}
			}
		}
		if pick < 0 {
			out = append(out, pos...)
			break
		}
		lit := pos[pick]
		out = append(out, lit)
		if lit.Pos != nil {
			bindAll(lit.Pos, bound)
		}
		pos = append(pos[:pick:pick], pos[pick+1:]...)
	}
	return Body{Literals: append(out, negs...)}
}

// isCheck reports whether a literal only tests: a comparison or a filter.
func isCheck(b *Base, lit Literal) bool {
	if lit.Compare != nil {
		return true
	}
	bi, ok := b.reg.Predicate(lit.Pos.Relation)
	return ok && !bi.IsGenerator()
}

// isProbe reports whether a literal reads a relation rather than calling a predicate. Once all its
// arguments are bound (checkReady) it only asks whether a tuple exists, so it ranks with the checks.
func isProbe(b *Base, lit Literal) bool {
	if lit.Pos == nil {
		return false
	}
	_, isPred := b.reg.Predicate(lit.Pos.Relation)
	return !isPred
}

// checkReady reports whether every variable a check reads is bound.
func checkReady(lit Literal, bound map[Var]bool) bool {
	var terms []Term
	if lit.Compare != nil {
		terms = []Term{lit.Compare.Left, lit.Compare.Right}
	} else {
		terms = lit.Pos.Args
	}
	for _, t := range terms {
		if t.Var != "" && !bound[t.Var] {
			return false
		}
	}
	return true
}

func count(flags []bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}
