package datalog

import (
	"sort"
	"strings"
)

// magic is SemiNaive's demand-driven rewrite, the magic-sets transformation, run after unfold and
// before plan. A derived relation called with bound arguments is evaluated only for the values the
// query demands, rather than in full and then filtered. Asked reach("v0", ?x), the rewrite derives
//
//	m_reach_bf("v0").
//	reach_bf(?a, ?b) :- m_reach_bf(?a), edge(?a, ?b);
//	reach_bf(?a, ?c) :- m_reach_bf(?a), reach_bf(?a, ?b), edge(?b, ?c);
//
// so only reach tuples starting at v0 are computed. The "bf" adornment records which arguments are
// bound (b) and free (f) at a call; the magic relation m_reach_bf holds the bound values demanded so
// far. A rule passes demand on to what it calls: in reach(?a, ?c) :- edge(?a, ?b), reach(?b, ?c),
// demand for ?a becomes demand for each ?b, m_reach_bf(?b) :- m_reach_bf(?a), edge(?a, ?b).
// Everything is still ordinary rules, so semi-naive evaluation, strata and planning apply unchanged.
//
// Right-linear recursion called from the goal with constants is factored instead (see factor): plain
// magic sets would derive a full answer set for every node the recursion passes through.
//
// Which arguments are bound at a call depends on the order a body runs in, so each body is ordered by
// the planner first, starting from what its head has bound. A call with nothing bound is left calling
// the original relation, which is evaluated in full as before.
//
// A relation is only rewritten when neither its rules nor anything they read, transitively, contains a
// negation: demand flowing through a negation can turn a stratified program into one that is not, and
// stratified magic sets are a separate piece of work (#34). Such a relation is evaluated in full.
//
// Magic tuples record what was asked, not what produced an answer, so their citations must not reach
// an answer: SemiNaive's fixpoint derives them without citations (see SemiNaive.materialize).
func magic(b *Base, q Query) Query {
	m := &magician{b: b, byHead: map[string][]Rule{}, done: map[string]bool{}}
	for _, r := range q.Rules {
		m.byHead[r.Head.Relation] = append(m.byHead[r.Head.Relation], r)
	}
	m.eligible = negationFree(q.Rules)
	out := q
	if len(out.Select) == 0 {
		out.Select = defaultSelect(q.Goal)
	}
	var goal []Literal
	pos, negs := splitNegations(planBody(b, q.Goal, nil).Literals)
	bound := map[Var]bool{}
	var prefix []Literal
	for _, lit := range pos {
		if lit.Pos != nil {
			if call, ok := m.factor(*lit.Pos, bound); ok {
				lit = Literal{Pos: &call, at: lit.at}
			} else if call, ok := m.call(*lit.Pos, bound, prefix, nil); ok {
				lit = Literal{Pos: &call, at: lit.at}
			}
			bindAll(lit.Pos, bound)
		}
		goal = append(goal, lit)
		prefix = append(prefix, lit)
	}
	out.Goal = Body{Literals: append(goal, negs...)}
	for len(m.queue) > 0 {
		a := m.queue[0]
		m.queue = m.queue[1:]
		m.adornRules(a.rel, a.adorn)
	}
	if len(m.rules) == 0 {
		return q
	}
	out.Rules = append(append([]Rule(nil), q.Rules...), m.rules...)
	return dropUnreached(q, out)
}

type magician struct {
	b        *Base
	byHead   map[string][]Rule
	eligible map[string]bool
	done     map[string]bool // adorned relations already queued
	queue    []adorned
	rules    []Rule // magic, adorned and factored rules, in the order made
	factored int    // goal calls factored so far, numbering their relations
}

type adorned struct{ rel, adorn string }

// call rewrites one call to a derived relation for demand, if it qualifies: it is eligible and binds at
// least one argument. It adds the magic rule passing that demand in, guard being the caller's own
// demand (nil in the goal) and prefix the literals that run before the call, and returns the call to
// the adorned relation.
func (m *magician) call(a Atom, bound map[Var]bool, prefix []Literal, guard *Atom) (Atom, bool) {
	if !m.eligible[a.Relation] {
		return a, false
	}
	adorn, demanded := adornment(a, bound)
	if !strings.Contains(adorn, "b") {
		return a, false
	}
	body := prefix
	if guard != nil {
		body = append([]Literal{{Pos: guard}}, prefix...)
	}
	m.rules = append(m.rules, Rule{
		Head: Atom{Relation: magicName(a.Relation, adorn), Args: demanded},
		Body: Body{Literals: append([]Literal(nil), body...)},
	})
	if key := adornedName(a.Relation, adorn); !m.done[key] {
		m.done[key] = true
		m.queue = append(m.queue, adorned{a.Relation, adorn})
	}
	return Atom{Relation: adornedName(a.Relation, adorn), Args: a.Args}, true
}

// adornRules makes the adorned version of each rule of rel: guarded by its magic relation, with the
// calls in its body rewritten for the demand they now receive.
func (m *magician) adornRules(rel, adorn string) {
	for _, r := range m.byHead[rel] {
		entry := map[Var]bool{}
		var demanded []Term
		for i, t := range r.Head.Args {
			if adorn[i] == 'b' {
				demanded = append(demanded, t)
				if t.Var != "" && t.Var != "_" {
					entry[t.Var] = true
				}
			}
		}
		guard := &Atom{Relation: magicName(rel, adorn), Args: demanded}
		lits := []Literal{{Pos: guard}}
		bound := map[Var]bool{}
		for v := range entry {
			bound[v] = true
		}
		var prefix []Literal
		for _, lit := range planBody(m.b, r.Body, entry).Literals {
			if lit.Pos != nil {
				if call, ok := m.call(*lit.Pos, bound, prefix, guard); ok {
					lit = Literal{Pos: &call, at: lit.at}
				}
				bindAll(lit.Pos, bound)
			}
			lits = append(lits, lit)
			prefix = append(prefix, lit)
		}
		m.rules = append(m.rules, Rule{
			Head:      Atom{Relation: adornedName(rel, adorn), Args: r.Head.Args},
			Body:      Body{Literals: lits},
			Hops:      r.Hops,
			HeadTypes: r.HeadTypes,
			text:      r.text,
		})
	}
}

// adornment is a call's pattern of bound (b) and free (f) arguments, with the bound terms.
func adornment(a Atom, bound map[Var]bool) (string, []Term) {
	var sb strings.Builder
	var demanded []Term
	for _, t := range a.Args {
		if t.Const != nil || (t.Var != "" && t.Var != "_" && bound[t.Var]) {
			sb.WriteByte('b')
			demanded = append(demanded, t)
		} else {
			sb.WriteByte('f')
		}
	}
	return sb.String(), demanded
}

// The names a rewrite gives its relations hold a byte no query can spell.
const magicPrefix = "\x00m:"

func magicName(rel, adorn string) string   { return magicPrefix + rel + "/" + adorn }
func adornedName(rel, adorn string) string { return rel + "\x00/" + adorn }
func isMagic(rel string) bool              { return strings.HasPrefix(rel, magicPrefix) }

// negationFree is every derived relation whose rules, and the rules of everything they read,
// transitively, contain no negated literal.
func negationFree(rules []Rule) map[string]bool {
	reads := map[string][]string{}
	negates := map[string]bool{}
	for _, r := range rules {
		h := r.Head.Relation
		if _, ok := reads[h]; !ok {
			reads[h] = nil
		}
		for _, l := range r.Body.Literals {
			switch {
			case l.Neg != nil:
				negates[h] = true
			case l.Pos != nil:
				reads[h] = append(reads[h], l.Pos.Relation)
			}
		}
	}
	heads := make([]string, 0, len(reads))
	for h := range reads {
		heads = append(heads, h)
	}
	sort.Strings(heads)
	out := map[string]bool{}
	for _, h := range heads {
		seen := map[string]bool{}
		stack := []string{h}
		clean := true
		for len(stack) > 0 && clean {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			if negates[n] {
				clean = false
			}
			stack = append(stack, reads[n]...)
		}
		out[h] = clean
	}
	return out
}
