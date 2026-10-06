package datalog

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/panyam/jaala/ns"
)

// The corpus checks jaala's evaluators against Naive, which can't tell when Naive itself is wrong.
// This checks Naive against Soufflé (#23): each generated program the two engines can both express is
// translated to Soufflé's language, run there, and its answer compared with Naive's. Soufflé isn't a
// Go dependency: the translator is test code, and the test runs the souffle binary when it's on PATH.
//
// The translation covers the generator's fragment, and says why when a program falls outside it
// (souffleUnsupported), so a skipped program is counted by reason rather than dropped:
//
//	edge/node/weight            .decl and .input; a node is a symbol, a weight a number
//	rules, not, _, constants    h(..) :- .., !r(..).
//	comparisons                 as written: Soufflé orders symbols by their bytes, as jaala orders text
//	count, sum, min, max        an aggregate over a helper relation of the body's bindings (every _
//	                            named, so a binding is a tuple) or of the group's distinct values
//	an aggregate goal with no   Soufflé's min and max give no row over nothing; jaala answers one row,
//	group column                count 0 and the rest empty, so that row is written out explicitly
//	order by, limit             applied in Go, with jaala's own order, to Soufflé's answer set
//	Bind                        one value as the constant it is in jaala; several as a relation
//	types                       by unification over every place a variable or constant sits, as
//	                            Soufflé checks them; a value jaala lets be both is outside

// souffleUnsupported is why a program is outside the fragment the translator covers.
type souffleUnsupported struct{ reason string }

func (u souffleUnsupported) Error() string { return "outside the Soufflé fragment: " + u.reason }

func unsupported(format string, args ...any) error {
	return souffleUnsupported{reason: fmt.Sprintf(format, args...)}
}

// sKind is a column's Soufflé type.
type sKind int

const (
	sUnknown sKind = iota
	sSymbol
	sNumber
)

func (k sKind) String() string {
	if k == sNumber {
		return "number"
	}
	return "symbol"
}

// souffleBase is the generator's graph, as Soufflé declares it.
var souffleBase = map[string][]sKind{
	"edge":   {sSymbol, sSymbol},
	"node":   {sSymbol},
	"weight": {sSymbol, sNumber},
}

// souffleTranslation is one program in Soufflé's language, its relations prefixed so several can share
// one run, and what reading its answer back needs.
type souffleTranslation struct {
	prefix string
	text   string
	q      Query
	cols   []sKind // the answer relation's columns
	// asText marks an aggregate goal with no group column, whose answer columns are written as
	// symbols so its row over nothing can hold jaala's empty value.
	asText bool
}

// translateSouffle writes q, with bind's values for goal variables, as a Soufflé program whose relations
// are prefixed with prefix. Facts are read from <prefix>edge.facts and so on.
func translateSouffle(q Query, bind map[Var][]ns.Value, prefix string) (souffleTranslation, error) {
	if q.Offset != 0 {
		return souffleTranslation{}, unsupported("offset")
	}
	tr := &souffler{prefix: prefix}
	body, one := bindGoalBody(q.Goal, bind)
	if err := tr.infer(q.Rules, body); err != nil {
		return souffleTranslation{}, err
	}
	for _, rel := range sortedKeys(souffleBase) {
		tr.decl(rel, tr.kinds[rel])
		fmt.Fprintf(&tr.b, ".input %s%s\n", prefix, rel)
	}
	for _, rel := range sortedKeys(tr.derived) {
		tr.decl(rel, tr.kinds[rel])
	}
	for _, v := range sortedKeys(bindsByName(bind)) {
		vals := bind[Var(v)]
		if len(vals) == 1 {
			continue // a constant (see bindGoalBody)
		}
		tr.decl("bind_"+v, []sKind{sSymbol})
		for _, val := range vals {
			fmt.Fprintf(&tr.b, "%sbind_%s(%s).\n", prefix, v, souffleConst(val))
		}
	}
	for i, r := range q.Rules {
		if err := tr.rule(r, tr.ruleVK[i]); err != nil {
			return souffleTranslation{}, err
		}
	}
	out, err := tr.goal(q, body, one)
	if err != nil {
		return souffleTranslation{}, err
	}
	out.prefix, out.text, out.q = prefix, tr.b.String(), q
	return out, nil
}

func bindsByName(bind map[Var][]ns.Value) map[string]bool {
	out := map[string]bool{}
	for v := range bind {
		out[string(v)] = true
	}
	return out
}

// bindGoalBody is the goal as jaala evaluates it with bind: a variable bound to one value is that
// constant (bindGoal), so it is written into the body, its answer column, and no group key, and an
// aggregate over nothing still answers jaala's one row, holding it. A variable bound to several joins
// a relation of them, bind_<var>, and stays a variable.
func bindGoalBody(goal Body, bind map[Var][]ns.Value) (Body, map[Var]ns.Value) {
	one := map[Var]ns.Value{}
	body := Body{Literals: slices.Clone(goal.Literals)}
	for _, v := range sortedKeys(bindsByName(bind)) {
		vals := bind[Var(v)]
		if len(vals) == 1 {
			one[Var(v)] = vals[0]
			continue
		}
		body.Literals = append(body.Literals, Literal{Pos: &Atom{Relation: "bind_" + v, Args: []Term{{Var: Var(v)}}}})
	}
	if len(one) > 0 {
		body = substBody(body, func(t Term) Term {
			if val, ok := one[t.Var]; ok {
				c := val
				return Term{Const: &c}
			}
			return t
		})
	}
	return body, one
}

// souffleDropNegation makes the translator write a negated literal as a positive one, a wrong
// translation TestSouffleCatchesAWrongTranslation uses to show the comparison can fail.
var souffleDropNegation bool

type souffler struct {
	prefix  string
	kinds   map[string][]sKind
	derived map[string]bool
	ruleVK  []map[Var]sKind // each rule's variables' types
	goalVK  map[Var]sKind
	b       strings.Builder
	aux     int
}

func (tr *souffler) decl(rel string, ks []sKind) {
	cols := make([]string, len(ks))
	for i, k := range ks {
		cols[i] = fmt.Sprintf("c%d:%s", i, k)
	}
	fmt.Fprintf(&tr.b, ".decl %s%s(%s)\n", tr.prefix, rel, strings.Join(cols, ", "))
}

// sTypes types a program as Soufflé does, by unification: every place a variable sits (a relation's
// column, the other side of a comparison) is one type, and a constant or an aggregate fixes it. A
// variable or column that would have to be both, which jaala allows, is outside the fragment.
type sTypes struct {
	parent map[string]string
	kind   map[string]sKind
	err    error
}

func (u *sTypes) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *sTypes) set(x string, k sKind) {
	r := u.find(x)
	switch had := u.kind[r]; {
	case k == sUnknown || had == k:
	case had == sUnknown:
		u.kind[r] = k
	case u.err == nil:
		u.err = unsupported("a value is used as both a symbol and a number")
	}
}

func (u *sTypes) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	ka, kb := u.kind[ra], u.kind[rb]
	u.parent[ra] = rb
	if ka != sUnknown {
		u.set(rb, ka)
	}
	_ = kb
}

func slot(rel string, i int) string { return fmt.Sprintf("%s#%d", rel, i) }

// constrain adds a body's literals: each atom's arguments to its relation's columns, and each
// comparison's sides to each other. scope names the body's variables apart from other bodies'.
func (u *sTypes) constrain(scope string, b Body) {
	term := func(t Term, node string) {
		switch {
		case t.Const != nil && t.Const.Num != nil:
			u.set(node, sNumber)
		case t.Const != nil:
			u.set(node, sSymbol)
		case t.Var != "" && t.Var != "_":
			u.union(scope+string(t.Var), node)
		}
	}
	for _, l := range b.Literals {
		a := l.Pos
		if a == nil {
			a = l.Neg
		}
		if a != nil {
			for i, t := range a.Args {
				term(t, slot(a.Relation, i))
			}
			continue
		}
		c := l.Compare
		left := scope + "cmp:" + c.Left.String()
		term(c.Left, left)
		term(c.Right, left)
	}
}

// infer types every derived relation's columns and every rule's and the goal's variables.
func (tr *souffler) infer(rules []Rule, goal Body) error {
	u := &sTypes{parent: map[string]string{}, kind: map[string]sKind{}}
	for rel, ks := range souffleBase {
		for i, k := range ks {
			u.set(slot(rel, i), k)
		}
	}
	tr.derived = map[string]bool{}
	arity := map[string]int{}
	for i, r := range rules {
		scope := fmt.Sprintf("r%d:", i)
		tr.derived[r.Head.Relation] = true
		arity[r.Head.Relation] = len(r.Head.Args)
		u.constrain(scope, r.Body)
		for j, a := range r.Head.Args {
			if a.Agg != nil {
				u.set(slot(r.Head.Relation, j), sNumber)
				if a.Agg.Func != "count" {
					u.set(scope+string(a.Agg.Var), sNumber)
				}
				continue
			}
			switch {
			case a.Const != nil && a.Const.Num != nil:
				u.set(slot(r.Head.Relation, j), sNumber)
			case a.Const != nil:
				u.set(slot(r.Head.Relation, j), sSymbol)
			case a.Var != "" && a.Var != "_":
				u.union(scope+string(a.Var), slot(r.Head.Relation, j))
			}
		}
	}
	for _, l := range goal.Literals {
		if l.Pos != nil && strings.HasPrefix(l.Pos.Relation, "bind_") {
			u.set(slot(l.Pos.Relation, 0), sSymbol)
		}
	}
	u.constrain("g:", goal)
	if u.err != nil {
		return u.err
	}
	kindOf := func(node string) sKind {
		if k := u.kind[u.find(node)]; k != sUnknown {
			return k
		}
		return sSymbol // nothing fixes it, so nothing can tell
	}
	tr.kinds = map[string][]sKind{}
	for rel, ks := range souffleBase {
		tr.kinds[rel] = ks
	}
	for rel, n := range arity {
		ks := make([]sKind, n)
		for i := range ks {
			ks[i] = kindOf(slot(rel, i))
		}
		tr.kinds[rel] = ks
	}
	vars := func(scope string, b Body, extra []Term) map[Var]sKind {
		vk := map[Var]sKind{}
		for _, v := range bodyVars(b, extra) {
			vk[v] = kindOf(scope + string(v))
		}
		return vk
	}
	tr.ruleVK = make([]map[Var]sKind, len(rules))
	for i, r := range rules {
		tr.ruleVK[i] = vars(fmt.Sprintf("r%d:", i), r.Body, r.Head.Args)
	}
	tr.goalVK = vars("g:", goal, nil)
	for _, l := range goal.Literals {
		if l.Pos != nil && strings.HasPrefix(l.Pos.Relation, "bind_") {
			tr.kinds[l.Pos.Relation] = []sKind{sSymbol}
		}
	}
	return nil
}

// bodyVars is every variable b and extra name.
func bodyVars(b Body, extra []Term) []Var {
	var out []Var
	add := func(t Term) {
		if t.Var != "" && t.Var != "_" && !slices.Contains(out, t.Var) {
			out = append(out, t.Var)
		}
		if t.Agg != nil && !slices.Contains(out, t.Agg.Var) {
			out = append(out, t.Agg.Var)
		}
	}
	for _, l := range b.Literals {
		switch {
		case l.Pos != nil:
			for _, t := range l.Pos.Args {
				add(t)
			}
		case l.Neg != nil:
			for _, t := range l.Neg.Args {
				add(t)
			}
		default:
			add(l.Compare.Left)
			add(l.Compare.Right)
		}
	}
	for _, t := range extra {
		add(t)
	}
	return out
}

// rule writes one rule: as it is, or for an aggregating head, over a helper relation of its body.
func (tr *souffler) rule(r Rule, vk map[Var]sKind) error {
	if !slices.ContainsFunc(r.Head.Args, func(t Term) bool { return t.Agg != nil }) {
		fmt.Fprintf(&tr.b, "%s :- %s.\n", tr.atom(r.Head), tr.body(r.Body))
		return nil
	}
	var keys []Var
	for _, a := range r.Head.Args {
		if a.Agg == nil && a.Var != "" {
			keys = append(keys, a.Var)
		}
	}
	if len(keys) == 0 {
		for _, a := range r.Head.Args {
			if a.Agg != nil && a.Agg.Func != "count" {
				return unsupported("a rule head %s with no group column", a.Agg.Func)
			}
		}
	}
	aggs, err := tr.aggregates(r.Body, vk, keys, r.Head.Args)
	if err != nil {
		return err
	}
	args := make([]string, len(r.Head.Args))
	for i, a := range r.Head.Args {
		if a.Agg != nil {
			args[i] = aggs.result[i]
		} else {
			args[i] = souffleTerm(a)
		}
	}
	fmt.Fprintf(&tr.b, "%s%s(%s) :- %s.\n", tr.prefix, r.Head.Relation, strings.Join(args, ", "), strings.Join(aggs.lits, ", "))
	return nil
}

// souffleAggs is the literals that read a head's or a goal's aggregates, each into a variable.
type souffleAggs struct {
	lits     []string       // the group's keys relation, then one aggregate relation per aggregate
	result   map[int]string // position -> the variable holding that aggregate
	nonEmpty string         // with no group key: a nullary relation holding when the body has a binding
}

// aggregates writes, for a body grouped by keys, a helper relation holding each binding (every _ given
// a name, so bindings that differ only there are separate tuples, as jaala counts them), the group
// keys, per distinct aggregate a relation of its group's distinct values, and per aggregate a relation
// of its value by group. Each aggregate gets a rule of its own, since Soufflé 2.5 crashes on a rule
// whose body is two aggregates and nothing else.
func (tr *souffler) aggregates(body Body, vk map[Var]sKind, keys []Var, terms []Term) (souffleAggs, error) {
	tr.aux++
	n := tr.aux
	named, cols, colKinds := tr.namedBody(body, vk)
	all := fmt.Sprintf("all%d", n)
	tr.decl(all, colKinds)
	fmt.Fprintf(&tr.b, "%s%s(%s) :- %s.\n", tr.prefix, all, joinVars(cols), tr.body(named))
	// local is v's name inside an aggregate's braces: the outer variable when v is a group key, which
	// the keys relation binds, else a variable of its own.
	local := func(v Var) string {
		if slices.Contains(keys, v) {
			return souffleVar(v)
		}
		return "L_" + string(v)
	}
	keyed := func(rel string, extra Var) string { // rel over the outer keys, extra as local names it
		args := make([]string, len(cols))
		for i, c := range cols {
			switch {
			case slices.Contains(keys, c):
				args[i] = souffleVar(c)
			case c == extra:
				args[i] = local(c)
			default:
				args[i] = "_"
			}
		}
		return fmt.Sprintf("%s%s(%s)", tr.prefix, rel, strings.Join(args, ", "))
	}
	out := souffleAggs{result: map[int]string{}}
	var group []string // what an aggregate's own rule starts from
	if len(keys) > 0 {
		keysRel := fmt.Sprintf("keys%d", n)
		tr.decl(keysRel, keyKinds(keys, vk))
		fmt.Fprintf(&tr.b, "%s%s(%s) :- %s.\n", tr.prefix, keysRel, joinVars(keys), keyed(all, ""))
		group = []string{fmt.Sprintf("%s%s(%s)", tr.prefix, keysRel, joinVars(keys))}
		out.lits = append(out.lits, group[0])
	} else {
		out.nonEmpty = fmt.Sprintf("%snonempty%d", tr.prefix, n)
		fmt.Fprintf(&tr.b, ".decl %s()\n%s() :- %s.\n", out.nonEmpty, out.nonEmpty, keyed(all, ""))
	}
	for i, t := range terms {
		a := t.Agg
		if a == nil {
			continue
		}
		if a.Func == "list" {
			return souffleAggs{}, unsupported("list")
		}
		if a.Func != "count" && vk[a.Var] != sNumber {
			return souffleAggs{}, unsupported("%s over symbols", a.Func)
		}
		over := keyed(all, a.Var) // every binding of the group: count counts them, sum adds their values
		if a.Distinct || a.Func == "min" || a.Func == "max" {
			// Over the group's distinct values. min and max are the same either way.
			dist := fmt.Sprintf("dist%d_%d", n, i)
			tr.decl(dist, append(keyKinds(keys, vk), vk[a.Var]))
			args := strings.Join(append(varsOf(keys), local(a.Var)), ", ")
			fmt.Fprintf(&tr.b, "%s%s(%s) :- %s.\n", tr.prefix, dist, args, keyed(all, a.Var))
			over = fmt.Sprintf("%s%s(%s)", tr.prefix, dist, args)
		}
		res := fmt.Sprintf("A%d_%d", n, i)
		expr := fmt.Sprintf("%s = count : { %s }", res, over)
		if a.Func != "count" {
			expr = fmt.Sprintf("%s = %s %s : { %s }", res, a.Func, local(a.Var), over)
		}
		aggRel := fmt.Sprintf("agg%d_%d", n, i)
		tr.decl(aggRel, append(keyKinds(keys, vk), sNumber))
		head := strings.Join(append(varsOf(keys), res), ", ")
		fmt.Fprintf(&tr.b, "%s%s(%s) :- %s.\n", tr.prefix, aggRel, head, strings.Join(append(slices.Clone(group), expr), ", "))
		out.lits = append(out.lits, fmt.Sprintf("%s%s(%s)", tr.prefix, aggRel, head))
		out.result[i] = res
	}
	return out, nil
}

func keyKinds(keys []Var, vk map[Var]sKind) []sKind {
	out := make([]sKind, len(keys))
	for i, k := range keys {
		out[i] = vk[k]
	}
	return out
}

// namedBody is body with every _ in a positive atom given a fresh variable, and the body's positive
// variables in first-seen order with their types: one tuple per binding.
func (tr *souffler) namedBody(body Body, vk map[Var]sKind) (Body, []Var, []sKind) {
	out := Body{Literals: make([]Literal, len(body.Literals))}
	var cols []Var
	var kinds []sKind
	seen := map[Var]bool{}
	w := 0
	for i, l := range body.Literals {
		out.Literals[i] = l
		if l.Pos == nil {
			continue
		}
		a := *l.Pos
		a.Args = slices.Clone(a.Args)
		ks := tr.kinds[a.Relation]
		for j, t := range a.Args {
			if t.Var == "_" {
				w++
				name := Var(fmt.Sprintf("w%d", w))
				a.Args[j] = Term{Var: name}
				vk[name] = ks[j]
				t = a.Args[j]
			}
			if t.Var != "" && !seen[t.Var] {
				seen[t.Var] = true
				cols = append(cols, t.Var)
				kinds = append(kinds, vk[t.Var])
			}
		}
		out.Literals[i] = Literal{Pos: &a}
	}
	return out, cols, kinds
}

// body writes a rule body's literals.
func (tr *souffler) body(b Body) string {
	var out []string
	for _, l := range b.Literals {
		switch {
		case l.Pos != nil:
			out = append(out, tr.atom(*l.Pos))
		case l.Neg != nil && souffleDropNegation:
			out = append(out, tr.atom(*l.Neg))
		case l.Neg != nil:
			out = append(out, "!"+tr.atom(*l.Neg))
		default:
			c := l.Compare
			out = append(out, souffleTerm(c.Left)+" "+c.Op+" "+souffleTerm(c.Right))
		}
	}
	return strings.Join(out, ", ")
}

func (tr *souffler) atom(a Atom) string {
	args := make([]string, len(a.Args))
	for i, t := range a.Args {
		args[i] = souffleTerm(t)
	}
	return fmt.Sprintf("%s%s(%s)", tr.prefix, a.Relation, strings.Join(args, ", "))
}

// goal writes the answer relation: the goal's projection, or its aggregates with having as a filter.
func (tr *souffler) goal(q Query, body Body, one map[Var]ns.Value) (souffleTranslation, error) {
	vk := tr.goalVK
	sel := q.Select
	if len(sel) == 0 {
		sel = defaultSelect(q.Goal)
	}
	hasAgg := slices.ContainsFunc(sel, func(t Term) bool { return t.Agg != nil })
	if !hasAgg {
		if len(q.Having) > 0 {
			return souffleTranslation{}, unsupported("having without an aggregate")
		}
		cols := make([]sKind, len(sel))
		head := make([]string, len(sel))
		for i, t := range sel {
			cols[i], head[i] = vk[t.Var], souffleTerm(t)
			if val, ok := one[t.Var]; ok {
				cols[i], head[i] = sSymbol, souffleConst(val)
			}
		}
		tr.decl("answer", cols)
		fmt.Fprintf(&tr.b, "%sanswer(%s) :- %s.\n.output %sanswer\n", tr.prefix, strings.Join(head, ", "), tr.body(body), tr.prefix)
		return souffleTranslation{cols: cols}, nil
	}
	var keys []Var
	for _, t := range sel {
		if _, bound := one[t.Var]; t.Agg == nil && !bound {
			keys = append(keys, t.Var)
		}
	}
	aggs, err := tr.aggregates(body, vk, keys, sel)
	if err != nil {
		return souffleTranslation{}, err
	}
	filter, err := tr.having(q.Having, sel, aggs)
	if err != nil {
		return souffleTranslation{}, err
	}
	args := make([]string, len(sel))
	cols := make([]sKind, len(sel))
	asText := len(keys) == 0
	for i, t := range sel {
		switch val, bound := one[t.Var]; {
		case t.Agg != nil && asText:
			args[i], cols[i] = "to_string("+aggs.result[i]+")", sSymbol
		case t.Agg != nil:
			args[i], cols[i] = aggs.result[i], sNumber
		case bound:
			args[i], cols[i] = souffleConst(val), sSymbol
		default:
			args[i], cols[i] = souffleVar(t.Var), vk[t.Var]
		}
	}
	tr.decl("answer", cols)
	lits := aggs.lits
	if asText {
		lits = append([]string{aggs.nonEmpty + "()"}, lits...)
	}
	fmt.Fprintf(&tr.b, "%sanswer(%s) :- %s.\n", tr.prefix, strings.Join(args, ", "), strings.Join(append(lits, filter...), ", "))
	if asText && len(q.Having) == 0 {
		// jaala answers one row over nothing: count 0, the other aggregates empty, and a bound column
		// its value.
		empty := make([]string, len(sel))
		for i, t := range sel {
			switch {
			case t.Agg == nil:
				empty[i] = souffleConst(one[t.Var])
			case t.Agg.Func == "count":
				empty[i] = `"0"`
			default:
				empty[i] = `""`
			}
		}
		fmt.Fprintf(&tr.b, "%sanswer(%s) :- !%s().\n", tr.prefix, strings.Join(empty, ", "), aggs.nonEmpty)
	}
	fmt.Fprintf(&tr.b, ".output %sanswer\n", tr.prefix)
	return souffleTranslation{cols: cols, asText: asText}, nil
}

// having writes each group filter as a constraint on the aggregate's variable. Over nothing jaala's
// row fails any having the generator writes (count 0, or an empty value, against at least 1), so the
// row over nothing is left out when there is one.
func (tr *souffler) having(hs []Compare, sel []Term, aggs souffleAggs) ([]string, error) {
	var out []string
	for _, h := range hs {
		i := slices.IndexFunc(sel, func(t Term) bool {
			return t.Agg != nil && h.Left.Agg != nil && *t.Agg == *h.Left.Agg
		})
		if i < 0 || h.Right.Const == nil || h.Right.Const.Num == nil {
			return nil, unsupported("having on an unselected aggregate or a non-number")
		}
		out = append(out, aggs.result[i]+" "+h.Op+" "+souffleTerm(h.Right))
	}
	return out, nil
}

func souffleVar(v Var) string { return "V_" + string(v) }

func souffleTerm(t Term) string {
	switch {
	case t.Const != nil:
		return souffleConst(*t.Const)
	case t.Var == "_" || t.Var == "":
		return "_"
	}
	return souffleVar(t.Var)
}

func souffleConst(v ns.Value) string {
	if v.Num != nil {
		return numberKey(*v.Num)
	}
	return strconv.Quote(v.S)
}

func joinVars(vs []Var) string { return strings.Join(varsOf(vs), ", ") }

func varsOf(vs []Var) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = souffleVar(v)
	}
	return out
}

func joinTerms(ts []Term) string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = souffleTerm(t)
	}
	return strings.Join(out, ", ")
}

// writeSouffleFacts writes the generator's graph as Soufflé fact files in dir, prefixed.
func writeSouffleFacts(dir, prefix string, src ns.Source) error {
	for rel := range souffleBase {
		var b strings.Builder
		for _, t := range src.Tuples(rel) {
			for i, v := range t.Vals {
				if i > 0 {
					b.WriteByte('\t')
				}
				b.WriteString(v.S)
			}
			b.WriteByte('\n')
		}
		if err := os.WriteFile(filepath.Join(dir, prefix+rel+".facts"), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// runSouffle runs the translations in one souffle invocation over the facts in dir, and returns each
// one's answer rows, in jaala's default order with the query's order by and limit applied.
func runSouffle(bin, dir string, trs []souffleTranslation) ([][]Row, error) {
	var prog strings.Builder
	for _, tr := range trs {
		prog.WriteString(tr.text)
	}
	dl := filepath.Join(dir, "prog.dl")
	if err := os.WriteFile(dl, []byte(prog.String()), 0o644); err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, "-w", "-F", dir, "-D", dir, dl).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("souffle: %v\n%s", err, out)
	}
	rows := make([][]Row, len(trs))
	for i, tr := range trs {
		data, err := os.ReadFile(filepath.Join(dir, tr.prefix+"answer.csv"))
		if err != nil {
			return nil, err
		}
		rows[i] = souffleRows(tr, string(data))
	}
	return rows, nil
}

// souffleRows reads one answer file as jaala rows, then orders and pages them as jaala would.
func souffleRows(tr souffleTranslation, data string) []Row {
	sel := tr.q.Select
	var rows []Row
	var lines []string
	if data != "" { // an empty file is no rows; "\n" is one row whose one column is empty text
		lines = strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	}
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		row := Row{Bind: map[Var]ns.Value{}}
		for i, t := range sel {
			f := ""
			if i < len(fields) {
				f = fields[i]
			}
			var v ns.Value
			switch {
			case tr.cols[i] == sNumber, tr.asText && t.Agg != nil && f != "":
				n, _ := strconv.ParseFloat(f, 64)
				v = ns.N(n)
			case tr.asText && t.Agg != nil:
				v = ns.Value{}
			default:
				v = ns.S(f)
			}
			row.Bind[colLabel(t)] = v
		}
		rows = append(rows, row)
	}
	rows = dedupSort(rows, selKeys(sel))
	orderRows(rows, tr.q.OrderBy)
	return page(rows, tr.q.Limit, tr.q.Offset)
}

// renderRows is an answer as text to compare: each row's columns, a number marked apart from text.
func renderRows(rows []Row, sel []Term) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		cells := make([]string, len(sel))
		for j, t := range sel {
			v := r.Bind[colLabel(t)]
			switch {
			case v.Num != nil:
				cells[j] = "n:" + v.S
			default:
				cells[j] = "s:" + v.S
			}
		}
		out[i] = strings.Join(cells, " ")
	}
	return out
}

// souffleCase is one generated program prepared for Soufflé: its translation and Naive's answer.
type souffleCase struct {
	c     genCase
	tr    souffleTranslation
	naive []string
}

// prepareSouffle parses and translates c, and runs it through Naive. It returns a reason instead for a
// program Naive refuses or can't finish within genBudget, or the translator doesn't cover.
func prepareSouffle(c genCase, prefix string) (souffleCase, string, error) {
	q, err := Parse(c.prog.String())
	if err != nil {
		return souffleCase{}, "", err
	}
	src := c.facts.source()
	b := baseFor(std(src))
	opts := c.opts()
	rows, err := (Naive{}).Eval(bg, q, b, append(opts, Budget(genBudget))...)
	var over *BudgetExceeded
	if errors.As(err, &over) {
		return souffleCase{}, "over budget", nil
	}
	if err != nil {
		return souffleCase{}, "Naive refuses it", nil // the corpus checks the evaluators refuse alike
	}
	bind := map[Var][]ns.Value{}
	for v, names := range c.prog.goal.bind {
		bind[Var(v)] = []ns.Value{}
		for _, n := range names {
			bind[Var(v)] = append(bind[Var(v)], ns.S(n))
		}
	}
	tr, err := translateSouffle(q, bind, prefix)
	var u souffleUnsupported
	if errors.As(err, &u) {
		return souffleCase{}, u.reason, nil
	}
	if err != nil {
		return souffleCase{}, "", err
	}
	return souffleCase{c: c, tr: tr, naive: renderRows(rows, tr.q.Select)}, "", nil
}

// souffleDiff runs one case alone through Soufflé and reports how its answer differs from Naive's.
func souffleDiff(bin string, sc souffleCase) (string, error) {
	dir, err := os.MkdirTemp("", "jaala-souffle")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := writeSouffleFacts(dir, sc.tr.prefix, sc.c.facts.source()); err != nil {
		return "", err
	}
	rows, err := runSouffle(bin, dir, []souffleTranslation{sc.tr})
	if err != nil {
		return "", err
	}
	return compareSouffle(sc, rows[0]), nil
}

func compareSouffle(sc souffleCase, rows []Row) string {
	got := renderRows(rows, sc.tr.q.Select)
	if slices.Equal(got, sc.naive) {
		return ""
	}
	return fmt.Sprintf("Soufflé answers differently from Naive on %s\n naive:   %v\n souffle: %v\n translation:\n%s", sc.c.prog, sc.naive, got, sc.tr.text)
}

// souffleRun is what comparing a range of seeds with Soufflé found.
type souffleRun struct {
	compared map[string]int // by shape, and "program" for all
	skipped  map[string]int // by reason
	diffs    []string       // each disagreement, shrunk
	refused  []string       // translations Soufflé refused
}

// compareSeeds runs seeds first..first+n-1 through Naive and Soufflé, in batches of programs run in
// parallel, and shrinks each disagreement.
func compareSeeds(t *testing.T, bin string, first int64, n int) souffleRun {
	t.Helper()
	run := souffleRun{compared: map[string]int{}, skipped: map[string]int{}}
	var cases []souffleCase
	for seed := first; seed < first+int64(n); seed++ {
		c := corpusCase(seed)
		sc, why, err := prepareSouffle(c, fmt.Sprintf("p%d_", seed))
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, c.prog)
		}
		if why != "" {
			run.skipped[why]++
			continue
		}
		cases = append(cases, sc)
	}
	const batch = 200
	type outcome struct {
		sc      souffleCase
		diff    string
		refused error
	}
	var batches [][]outcome
	for start := 0; start < len(cases); start += batch {
		part := cases[start:min(start+batch, len(cases))]
		batches = append(batches, make([]outcome, len(part)))
		for i, sc := range part {
			batches[len(batches)-1][i].sc = sc
		}
	}
	sem := make(chan struct{}, max(1, runtime.NumCPU()/2))
	var wg sync.WaitGroup
	for _, outs := range batches {
		wg.Add(1)
		go func(outs []outcome) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			dir, err := os.MkdirTemp("", "jaala-souffle")
			if err != nil {
				for i := range outs {
					outs[i].refused = err
				}
				return
			}
			defer os.RemoveAll(dir)
			trs := make([]souffleTranslation, len(outs))
			for i, o := range outs {
				trs[i] = o.sc.tr
				if err := writeSouffleFacts(dir, o.sc.tr.prefix, o.sc.c.facts.source()); err != nil {
					outs[i].refused = err
				}
			}
			results, err := runSouffle(bin, dir, trs)
			for i := range outs {
				if err != nil {
					// One program Soufflé refuses fails the whole run: run each alone, so the rest are
					// still compared and the refused one is named.
					outs[i].diff, outs[i].refused = souffleDiff(bin, outs[i].sc)
				} else {
					outs[i].diff = compareSouffle(outs[i].sc, results[i])
				}
			}
		}(outs)
	}
	wg.Wait()
	for _, outs := range batches {
		for _, o := range outs {
			switch {
			case o.refused != nil:
				run.refused = append(run.refused, fmt.Sprintf("%s: %v", o.sc.c.prog, o.refused))
			case o.diff == "":
				for _, s := range o.sc.c.prog.shapes {
					run.compared[s]++
				}
				run.compared["program"]++
			default:
				prefix := o.sc.tr.prefix
				small, d := shrinkWhile(o.sc.c, o.diff, func(s genCase) (bool, string) {
					ssc, why, err := prepareSouffle(s, prefix)
					if err != nil || why != "" {
						return false, ""
					}
					d, err := souffleDiff(bin, ssc)
					return err == nil && d != "", d
				})
				seed := strings.TrimSuffix(strings.TrimPrefix(prefix, "p"), "_")
				run.diffs = append(run.diffs, fmt.Sprintf("seed %s disagrees with Soufflé; replay with JAALA_GEN_SEED=%s\n%s\n %s", seed, seed, small, d))
			}
		}
	}
	return run
}

// souffleBin is the souffle binary on PATH, or skips t without one, unless JAALA_SOUFFLE=require.
func souffleBin(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("souffle")
	if err != nil {
		if os.Getenv("JAALA_SOUFFLE") == "require" {
			t.Fatal("souffle is not on PATH, and JAALA_SOUFFLE=require")
		}
		t.Skip("souffle is not on PATH (JAALA_SOUFFLE=require makes this an error)")
	}
	return bin
}

// TestSouffleAgreesWithNaive runs the generated corpus through Soufflé and Naive and requires the same
// answers (#23). It skips when souffle isn't on PATH, unless JAALA_SOUFFLE=require, which CI's souffle
// job sets. JAALA_SOUFFLE_SEEDS sets the seed count, and JAALA_GEN_SEED replays one.
func TestSouffleAgreesWithNaive(t *testing.T) {
	bin := souffleBin(t)
	first, seeds := int64(1), envInt("JAALA_SOUFFLE_SEEDS", 300)
	if s := envInt("JAALA_GEN_SEED", 0); s > 0 {
		first, seeds = int64(s), 1
	}
	run := compareSeeds(t, bin, first, seeds)
	for _, r := range run.refused {
		t.Errorf("Soufflé refused the translation of %s", r)
	}
	for _, d := range run.diffs {
		t.Error(d)
	}
	var counts, reasons []string
	for _, s := range sortedKeys(run.compared) {
		counts = append(counts, fmt.Sprintf("%s %d", s, run.compared[s]))
	}
	for _, r := range sortedKeys(run.skipped) {
		reasons = append(reasons, fmt.Sprintf("%s %d", r, run.skipped[r]))
	}
	t.Logf("%d seeds\n compared with Soufflé: %s\n skipped: %s\n disagreements: %d", seeds, strings.Join(counts, ", "), strings.Join(reasons, ", "), len(run.diffs))
	if seeds >= 50 {
		for _, s := range []string{"program", "recursion", "negation", "head aggregate", "goal aggregate", "bound goal"} {
			if run.compared[s] == 0 {
				t.Errorf("control: no %s program was compared with Soufflé", s)
			}
		}
	}
}

// A translation that drops negation answers differently, and the comparison finds it and shrinks it
// to a program of a rule or two: it can fail, and its report is readable.
func TestSouffleCatchesAWrongTranslation(t *testing.T) {
	bin := souffleBin(t)
	souffleDropNegation = true
	defer func() { souffleDropNegation = false }()
	run := compareSeeds(t, bin, 1, 100)
	if len(run.diffs) == 0 {
		t.Fatal("dropping negation went unnoticed over 100 seeds")
	}
	small := run.diffs[0]
	if rules := strings.Count(small[strings.Index(small, "shrunk program:"):strings.Index(small, "facts:")], ":-"); rules > 2 {
		t.Errorf("the disagreement shrank to %d rules, want at most 2:\n%s", rules, small)
	}
	if !strings.Contains(small, "not ") {
		t.Errorf("the shrunk program has no negation left to drop:\n%s", small)
	}
}

// corpusCase is the generated corpus's program for seed, as TestGeneratedProgramsAgree builds it.
func corpusCase(seed int64) genCase {
	rnd := rand.New(rand.NewSource(seed))
	facts := genGraph(rnd)
	return genCase{prog: (&generator{rnd: rnd, nodes: facts.n}).program(), facts: facts}
}

// The translation, checked without Soufflé so CI's main job covers it: a rule, negation and a
// comparison as written; a set-bound variable as a relation of its values; a count over bindings with
// each _ named; an aggregate goal with no group column writing jaala's row over nothing; and each
// reason a program is left out.
func TestSouffleTranslation(t *testing.T) {
	tr := func(text string, bind map[Var][]ns.Value) (string, error) {
		q, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		out, err := translateSouffle(q, bind, "p_")
		return out.text, err
	}
	for _, c := range []struct {
		text string
		bind map[Var][]ns.Value
		want []string
	}{
		{`edge(?a, ?b), ?a < ?b => ?a`, nil, []string{"p_answer(V_a) :- p_edge(V_a, V_b), V_a < V_b."}},
		// r's second column is typed by the goal's constant alone, as Soufflé would.
		{`r(?a, ?b) :- node(?a), r(?a, ?b); r(?a, 2) => ?a`, nil, []string{".decl p_r(c0:symbol, c1:number)"}},
		{`r(?a, ?b) :- edge(?a, ?b), not node(?b), ?a != "v1"; r(?x, ?y) => ?y`, nil, []string{
			".decl p_r(c0:symbol, c1:symbol)",
			`p_r(V_a, V_b) :- p_edge(V_a, V_b), !p_node(V_b), V_a != "v1".`,
			"p_answer(V_y) :- p_r(V_x, V_y).",
			".output p_answer",
		}},
		{`edge(?a, ?b) => ?b`, map[Var][]ns.Value{"a": {ns.S("v1"), ns.S("v2")}}, []string{
			`p_bind_a("v1").`, `p_bind_a("v2").`, "p_answer(V_b) :- p_edge(V_a, V_b), p_bind_a(V_a).",
		}},
		{`edge(?a, ?b) => ?a, ?b`, map[Var][]ns.Value{"a": {ns.S("v1")}}, []string{
			`p_answer("v1", V_b) :- p_edge("v1", V_b).`,
		}},
		{`weight(?n, ?w) => ?n, sum(?w)`, map[Var][]ns.Value{"n": {ns.S("v3")}}, []string{
			`p_answer("v3", "") :- !p_nonempty1().`,
		}},
		{`edge(?a, _) => ?a, count(?a)`, nil, []string{
			"p_all1(V_a, V_w1) :- p_edge(V_a, V_w1).",
			"A1_1 = count : { p_all1(V_a, _) }",
		}},
		{`weight(_, ?w) => sum(distinct ?w), max(?w)`, nil, []string{
			"A1_0 = sum L_w : { p_dist1_0(L_w) }",
			"A1_1 = max L_w : { p_dist1_1(L_w) }",
			"p_answer(to_string(A1_0), to_string(A1_1)) :- p_nonempty1(),",
			`p_answer("", "") :- !p_nonempty1().`,
		}},
	} {
		got, err := tr(c.text, c.bind)
		if err != nil {
			t.Errorf("%s: %v", c.text, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: no %q in\n%s", c.text, w, got)
			}
		}
	}
	for _, c := range []struct{ text, reason string }{
		{`weight(?n, ?w), ?n < ?w => ?n`, "a value is used as both a symbol and a number"},
		{`node(?n) => list(?n)`, "list"},
		{`node(?n) => min(?n)`, "min over symbols"},
		{`r(?x) :- node(?x); r(?w) :- weight(_, ?w); r(?x) => ?x`, "a value is used as both a symbol and a number"},
		{`r(?a, ?b, ?a) :- edge(?a, _), weight(_, ?b); r(?x, ?y, ?y) => ?x`, "a value is used as both a symbol and a number"},

		{`r(sum(?w)) :- weight(_, ?w); r(?s) => ?s`, "a rule head sum with no group column"},
		{`node(?n) => ?n limit 1 offset 1`, "offset"},
	} {
		_, err := tr(c.text, nil)
		var u souffleUnsupported
		if !errors.As(err, &u) || u.reason != c.reason {
			t.Errorf("%s: %v; want it left out for %q", c.text, err, c.reason)
		}
	}
}
