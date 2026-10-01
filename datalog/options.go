package datalog

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/panyam/jaala/ns"
)

// An Option adjusts one Eval: Bind gives goal variables values from the host, Budget limits its work.
type Option func(*evalOptions)

type evalOptions struct {
	bind    map[Var]ns.Value
	budget  int64
	witness bool
}

// Bind gives goal variables values from the host, so a parameterized query needs no program text built
// per request: Eval(ctx, q, b, Bind(map[Var]ns.Value{"target": ns.S(id)})). A bound variable is exactly
// a constant written in the goal, so inlining, demand (magic sets) and planning all start from it,
// and it stays an answer column, holding its value in every row. Rules are not affected: their
// variables are their own. Binding a variable the goal does not use is an error.
func Bind(values map[Var]ns.Value) Option {
	return func(o *evalOptions) {
		if o.bind == nil {
			o.bind = map[Var]ns.Value{}
		}
		for v, val := range values {
			o.bind[v] = val
		}
	}
}

// Budget limits one Eval to maxWork units of the work Base.Work counts (candidate comparisons, and
// each solution a generator emits). Exceeding it stops the Eval with a *BudgetExceeded. The budget is
// the Eval's own: concurrent Evals on one Base do not spend each other's.
func Budget(maxWork int64) Option {
	return func(o *evalOptions) { o.budget = maxWork }
}

// BudgetExceeded is the error an Eval stops with when its work passes its Budget, so a host can tell a
// query that ran too long from one that is wrong.
type BudgetExceeded struct {
	Work, Limit int64
}

func (e *BudgetExceeded) Error() string {
	return fmt.Sprintf("query: evaluation stopped: work passed its budget of %d (at %d)", e.Limit, e.Work)
}

// evalRun is one Eval's own state, carried on its copy of the Base: its context, and the work it has
// done against its budget.
type evalRun struct {
	ctx     context.Context
	budget  int64
	used    int64
	witness bool // record witnesses (see Witnesses)
}

// ctxCheckEvery is how many units of work pass between checks of the context. Checking on every
// comparison would cost more than the joins it guards; this keeps a cancelled Eval stopping within
// microseconds of work.
const ctxCheckEvery = 1024

// step records one unit of work for this Eval and reports whether it must stop: its budget is spent,
// or its context is done.
func (r *evalRun) step() error {
	if r == nil {
		return nil
	}
	r.used++
	if r.budget > 0 && r.used > r.budget {
		return &BudgetExceeded{Work: r.used, Limit: r.budget}
	}
	if r.used%ctxCheckEvery == 0 {
		return r.done()
	}
	return nil
}

// done reports whether the Eval's context has ended, as the error the Eval returns.
func (r *evalRun) done() error {
	if r == nil || r.ctx == nil {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		return fmt.Errorf("query: evaluation stopped: %w", err)
	}
	return nil
}

func (r *evalRun) context() context.Context {
	if r == nil || r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

// bindGoal substitutes the host's values for goal variables, and returns the goal's answer columns as
// written, so a bound variable is still a column. It refuses a variable the goal does not use.
func bindGoal(q Query, bind map[Var]ns.Value) (Query, []Term, error) {
	sel := q.Select
	if len(sel) == 0 {
		sel = defaultSelect(q.Goal)
	}
	if len(bind) == 0 {
		return q, sel, nil
	}
	used := map[Var]bool{}
	for _, l := range q.Goal.Literals {
		for _, t := range literalTerms(l) {
			used[t.Var] = true
		}
	}
	var unknown []string
	for v := range bind {
		if !used[v] || v == "_" {
			unknown = append(unknown, "?"+string(v))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return q, nil, fmt.Errorf("query: cannot bind %s: the goal does not use it", strings.Join(unknown, ", "))
	}
	sub := func(t Term) Term {
		if val, ok := bind[t.Var]; ok && t.Var != "" {
			c := val
			return Term{Const: &c}
		}
		return t
	}
	out := q
	out.Goal = Body{Literals: make([]Literal, len(q.Goal.Literals))}
	for i, l := range q.Goal.Literals {
		switch {
		case l.Pos != nil:
			a := substAtom(*l.Pos, sub)
			l = Literal{Pos: &a}
		case l.Neg != nil:
			a := substAtom(*l.Neg, sub)
			l = Literal{Neg: &a}
		case l.Compare != nil:
			c := *l.Compare
			c.Left, c.Right = sub(c.Left), sub(c.Right)
			l = Literal{Compare: &c}
		}
		out.Goal.Literals[i] = l
	}
	out.Select = nil
	for _, t := range sel {
		if _, ok := bind[t.Var]; ok && t.Var != "" && t.Agg == nil {
			continue // filled in after evaluation: the same value in every row
		}
		out.Select = append(out.Select, t)
	}
	return out, sel, nil
}

func substAtom(a Atom, sub func(Term) Term) Atom {
	out := Atom{Relation: a.Relation, Args: make([]Term, len(a.Args))}
	for i, t := range a.Args {
		out.Args[i] = sub(t)
	}
	return out
}

// literalTerms is every term a literal mentions.
func literalTerms(l Literal) []Term {
	switch {
	case l.Pos != nil:
		return l.Pos.Args
	case l.Neg != nil:
		return l.Neg.Args
	case l.Compare != nil:
		return []Term{l.Compare.Left, l.Compare.Right}
	}
	return nil
}
