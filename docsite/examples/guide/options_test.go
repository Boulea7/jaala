package guide_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

func base() *datalog.Base {
	src := source()
	v, err := vocabulary(src)
	if err != nil {
		panic(err)
	}
	b, err := datalog.NewBase(v, src)
	if err != nil {
		panic(err)
	}
	return b
}

// Bind gives a goal variable its value from Go. The answer still has the
// column, filled with the bound value, and binding a variable the goal
// doesn't use is an error rather than a silent no-op.
func Example_bind() {
	b, ctx := base(), context.Background()
	q := datalog.MustParse(`deps.depends_on(?p, ?d) => ?p, ?d`)
	rows, err := datalog.SemiNaive{}.Eval(ctx, q, b,
		datalog.Bind(map[datalog.Var][]ns.Value{"p": {ns.S("api")}}))
	for _, r := range rows {
		fmt.Println(r.Bind["p"].S, r.Bind["d"].S)
	}
	_, err = datalog.SemiNaive{}.Eval(ctx, q, b,
		datalog.Bind(map[datalog.Var][]ns.Value{"x": {ns.S("api")}}))
	fmt.Println(err)
	// Output:
	// api log
	// api util
	// query: cannot bind ?x: the goal does not use it
}

// Bound to several values, a variable ranges over them, so one Eval asks
// about every package in the set, and an aggregate counts across all of
// them. api and log both depend on util, and it's counted once.
func Example_bindSet() {
	b, ctx := base(), context.Background()
	set := datalog.Bind(map[datalog.Var][]ns.Value{"p": {ns.S("api"), ns.S("log")}})
	q := datalog.MustParse(`deps.depends_on(?p, ?d) => ?p, ?d`)
	rows, _ := datalog.SemiNaive{}.Eval(ctx, q, b, set)
	for _, r := range rows {
		fmt.Println(r.Bind["p"].S, r.Bind["d"].S)
	}
	q = datalog.MustParse(`deps.depends_on(?p, ?d) => count(distinct ?d)`)
	rows, _ = datalog.SemiNaive{}.Eval(ctx, q, b, set)
	fmt.Println(rows[0].Bind["count(distinct d)"].S)
	// Output:
	// api log
	// api util
	// log util
	// 2
}

// Budget caps the work one Eval may do, and Work says how much it did, so
// a host can size the budget from what its real queries need.
func Example_budget() {
	b, ctx := base(), context.Background()
	q := datalog.MustParse(`deps.depends_on(?p, ?d) => ?p, ?d`)
	rows, err := datalog.SemiNaive{}.Eval(ctx, q, b, datalog.Budget(10_000))
	fmt.Println(len(rows), "rows, error:", err)

	_, err = datalog.SemiNaive{}.Eval(ctx, q, b, datalog.Budget(3))
	var over *datalog.BudgetExceeded
	fmt.Println(errors.As(err, &over), "limit", over.Limit)
	// Output:
	// 6 rows, error: <nil>
	// true limit 3
}

// The context reaches every Eval, so a host can stop a query that's taking
// too long, or whose caller went away. The error wraps the context's own.
func Example_cancel() {
	b := base()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := datalog.MustParse(`deps.depends_on(?p, ?d) => ?p, ?d`)
	_, err := datalog.SemiNaive{}.Eval(ctx, q, b)
	fmt.Println(errors.Is(err, context.Canceled))
	// Output:
	// true
}

// Explain fills a report of what an Eval did: each derived relation and
// how it was evaluated, each body in the order it ran, and where the work
// went. Report.String prints all of it; here, the relations and the goal.
func Example_explain() {
	b, ctx := base(), context.Background()
	var r datalog.Report
	q := datalog.MustParse(`deps.depends_on("app", ?d) => ?d`)
	_, err := datalog.SemiNaive{}.Eval(ctx, q, b, datalog.Explain(&r))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, rr := range r.Relations {
		fmt.Println(rr.Relation, rr.How, rr.Adornments, rr.Tuples, "tuples")
	}
	fmt.Println("goal:", r.Goal.Ran)
	for _, l := range r.Goal.Literals {
		fmt.Println(" ", l.Literal, l.Access, "passed", l.Passed)
	}
	// Output:
	// deps._step demand [bf] 3 tuples
	// deps.depends_on demand [bf] 3 tuples
	// goal: deps.depends_on/bf("app", ?d)
	//   deps.depends_on/bf("app", ?d) [derived scan] passed 3
}
