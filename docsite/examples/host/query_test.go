package host_test

import (
	"context"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// A query is parsed once and run as often as needed. Bind gives a goal
// variable its value from the host, so the query text stays fixed while the
// package changes, and Budget caps the work one evaluation may do.
func Example() {
	b, err := base(source())
	if err != nil {
		fmt.Println(err)
		return
	}
	q := datalog.MustParse(`deps.depends_on(?p, ?d) => ?d`)
	for _, pkg := range []string{"api", "log"} {
		rows, err := datalog.SemiNaive{}.Eval(context.Background(), q, b,
			datalog.Bind(map[datalog.Var]ns.Value{"p": ns.S(pkg)}),
			datalog.Budget(100_000))
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, r := range rows {
			fmt.Println(pkg, "depends on", r.Bind["d"].S, r.Cites)
		}
	}
	// Output:
	// api depends on auth [imports.txt:2]
	// api depends on log [imports.txt:3]
	// api depends on util [imports.txt:3 imports.txt:5]
	// log depends on util [imports.txt:5]
}
