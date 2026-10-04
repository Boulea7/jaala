package host_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/panyam/jaala/datalog"
)

// An error is either the query's own fault, whose text starts with "query:" and is safe to show to
// whoever wrote the query, or a BudgetExceeded, which a host can test for and handle on its own.
func Example_errors() {
	b, err := base(source())
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()
	_, err = datalog.SemiNaive{}.Eval(ctx, datalog.MustParse(`deps.depends_on(?p) => ?p`), b)
	fmt.Println(err)

	_, err = datalog.SemiNaive{}.Eval(ctx, datalog.MustParse(`deps.depends_on(?p, ?d) => ?p, ?d`), b, datalog.Budget(5))
	var over *datalog.BudgetExceeded
	if errors.As(err, &over) {
		fmt.Println("stopped at the budget of", over.Limit)
	}
	// Output:
	// query: relation "deps.depends_on" takes 2 args, got 1
	// stopped at the budget of 5
}
