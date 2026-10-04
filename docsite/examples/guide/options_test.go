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
		datalog.Bind(map[datalog.Var]ns.Value{"p": ns.S("api")}))
	for _, r := range rows {
		fmt.Println(r.Bind["p"].S, r.Bind["d"].S)
	}
	_, err = datalog.SemiNaive{}.Eval(ctx, q, b,
		datalog.Bind(map[datalog.Var]ns.Value{"x": ns.S("api")}))
	fmt.Println(err)
	// Output:
	// api log
	// api util
	// query: cannot bind ?x: the goal does not use it
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
