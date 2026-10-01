package datalog

import (
	"sort"
	"strconv"
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
// The literals that run before a call (its prefix) are what the magic rule passing demand into it
// reads, and the rule they came from reads them too. So that they run once, a prefix that does more
// than read the guard is stored in a supplementary relation (supplementary magic sets, #54), which
// both read:
//
//	s1(?a, ?b) :- m_reach_bf(?a), edge(?a, ?b);
//	m_reach_bf(?b) :- s1(?a, ?b);
//	reach_bf(?a, ?c) :- s1(?a, ?b), reach_bf(?b, ?c);
//
// Right-linear recursion called from the goal with constants is factored instead (see factor): plain
// magic sets would derive a full answer set for every node the recursion passes through.
//
// Which arguments are bound at a call depends on the order a body runs in, so each body is ordered by
// the planner first, starting from what its head has bound. A call with nothing bound is left calling
// the original relation, which is evaluated in full as before.
//
// Demand passes through relations that use negation (#34), and into a negated call: in
// nocov(?c) :- p(?c), not cov(?c), demand for nocov becomes demand for cov at the ?c that reach the
// negation. Demand into a negation can make a stratified program unstratifiable (a recursive caller's
// demand for cov would depend on the caller, which negates cov), so when the rewritten program does not
// stratify, the rewrite is made again with negated calls reading their relations in full. That one is
// always stratified: the rules it adds read the original relations only positively or as the program
// already did, and no original relation reads a rewritten one.
//
// Magic tuples record what was asked, not what produced an answer, so their citations must not reach
// an answer: SemiNaive's fixpoint derives them without citations (see SemiNaive.materialize). A
// supplementary tuple is a prefix's result, so it keeps its citations, and under Witnesses the
// witnesses of the literals it stands for (see idbTuple.parts).
func magic(b *Base, q Query) Query {
	out := magicWith(b, q, true)
	if _, err := stratify(out.Rules, derivedArity(out.Rules)); err != nil {
		return magicWith(b, q, false)
	}
	return out
}

// magicWith makes the rewrite, pushing demand into negated calls when intoNeg is set (see magic).
func magicWith(b *Base, q Query, intoNeg bool) Query {
	m := &magician{b: b, byHead: map[string][]Rule{}, done: map[string]bool{}, intoNeg: intoNeg}
	for _, r := range q.Rules {
		m.byHead[r.Head.Relation] = append(m.byHead[r.Head.Relation], r)
	}
	out := q
	if len(out.Select) == 0 {
		out.Select = defaultSelect(q.Goal)
	}
	// A supplementary relation is a set, so the goal keeps its own prefix when an aggregate counts
	// its bindings (see countsBindings).
	supply := !countsBindings(out.Select, q.Having)
	out.Goal = Body{Literals: m.body(nil, planBody(b, q.Goal, nil).Literals, nil, supply)}
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

// derivedArity is the arity of each relation the rules derive, as stratify takes it.
func derivedArity(rules []Rule) map[string]int {
	out := map[string]int{}
	for _, r := range rules {
		out[r.Head.Relation] = len(r.Head.Args)
	}
	return out
}

type magician struct {
	b        *Base
	byHead   map[string][]Rule
	intoNeg  bool            // push demand into negated calls
	done     map[string]bool // adorned relations already queued
	queue    []adorned
	rules    []Rule // magic, supplementary, adorned and factored rules, in the order made
	factored int    // goal calls factored so far, numbering their relations
	supplied int    // supplementary relations made so far, numbering them
}

type adorned struct{ rel, adorn string }

// body rewrites a planned body for demand: guard is the demand it runs under (nil in the goal), entry
// what that binds. Each call to a derived relation that binds an argument becomes a call to the adorned
// relation, with a magic rule passing the demand in; a goal call may be factored instead. When supply
// is set, a prefix that does more than read demand is stored once in a supplementary relation, which
// the magic rule and the rest of the body both read.
func (m *magician) body(guard *Atom, lits []Literal, entry map[Var]bool, supply bool) []Literal {
	bound := map[Var]bool{}
	for v := range entry {
		bound[v] = true
	}
	var front []Literal // what runs before the next literal, from the guard or the last supplementary relation
	if guard != nil {
		front = []Literal{{Pos: guard}}
	}
	demand := func(a Atom) (Atom, bool) {
		if guard == nil {
			if call, ok := m.factor(a, bound); ok {
				return call, true
			}
		}
		if !m.wants(a, bound) {
			return a, false
		}
		if supply && doesWork(front) {
			front = []Literal{m.supplement(front, bound)}
		}
		return m.call(a, bound, front), true
	}
	pos, negs := splitNegations(lits)
	for _, lit := range pos {
		if lit.Pos != nil {
			if call, ok := demand(*lit.Pos); ok {
				lit = Literal{Pos: &call, at: lit.at}
			}
			bindAll(lit.Pos, bound)
		}
		front = append(front, lit)
	}
	var outNegs []Literal
	for _, lit := range negs {
		if m.intoNeg && m.wants(*lit.Neg, bound) {
			call, _ := demand(*lit.Neg)
			lit = Literal{Neg: &call, at: lit.at}
		}
		outNegs = append(outNegs, lit)
	}
	return append(front, outNegs...)
}

// wants reports whether a call can be rewritten for demand: it reads a derived relation and binds at
// least one argument.
func (m *magician) wants(a Atom, bound map[Var]bool) bool {
	if _, derived := m.byHead[a.Relation]; !derived {
		return false
	}
	adorn, _ := adornment(a, bound)
	return strings.Contains(adorn, "b")
}

// call adds the magic rule passing a call's demand in, reading front (the guard and the literals that
// run before the call), and returns the call to the adorned relation.
func (m *magician) call(a Atom, bound map[Var]bool, front []Literal) Atom {
	adorn, demanded := adornment(a, bound)
	m.rules = append(m.rules, Rule{
		Head: Atom{Relation: magicName(a.Relation, adorn), Args: demanded},
		Body: Body{Literals: append([]Literal(nil), front...)},
	})
	if key := adornedName(a.Relation, adorn); !m.done[key] {
		m.done[key] = true
		m.queue = append(m.queue, adorned{a.Relation, adorn})
	}
	return Atom{Relation: adornedName(a.Relation, adorn), Args: a.Args}
}

// supplement stores front in a new supplementary relation over every variable bound so far, and
// returns the literal reading it.
func (m *magician) supplement(front []Literal, bound map[Var]bool) Literal {
	var vars []string
	for v := range bound {
		if v != "_" {
			vars = append(vars, string(v))
		}
	}
	sort.Strings(vars)
	head := Atom{Relation: supPrefix + strconv.Itoa(m.supplied)}
	m.supplied++
	for _, v := range vars {
		head.Args = append(head.Args, Term{Var: Var(v)})
	}
	m.rules = append(m.rules, Rule{Head: head, Body: Body{Literals: append([]Literal(nil), front...)}})
	return Literal{Pos: &head}
}

// doesWork reports whether front reads anything besides demand, so storing it once saves work.
func doesWork(front []Literal) bool {
	for _, l := range front {
		if l.Pos != nil && !isGuard(l.Pos.Relation) {
			return true
		}
	}
	return false
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
		m.rules = append(m.rules, Rule{
			Head:      Atom{Relation: adornedName(rel, adorn), Args: r.Head.Args},
			Body:      Body{Literals: m.body(guard, planBody(m.b, r.Body, entry).Literals, entry, true)},
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

// supPrefix names a supplementary relation (see magic).
const supPrefix = "\x00s:"

func isSupplementary(rel string) bool { return strings.HasPrefix(rel, supPrefix) }

// isGuard reports whether rel is what a rewritten body starts from: a magic relation, a supplementary
// one, or a factored reachable set. A rule the rewrite guards with one reads it first.
func isGuard(rel string) bool {
	return isMagic(rel) || isSupplementary(rel) || strings.Contains(rel, fromSep)
}
