package host_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/panyam/jaala/datalog"
)

// Witnesses records how each answer was derived: a tree of the rules that fired and the facts at its
// leaves, in the order the rules are written.
func Example_witnesses() {
	b, err := base(source())
	if err != nil {
		fmt.Println(err)
		return
	}
	q := datalog.MustParse(`deps.depends_on("app", "util")`)
	rows, err := datalog.SemiNaive{}.Eval(context.Background(), q, b, datalog.Witnesses())
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, w := range rows[0].Witness {
		show(w, 0)
	}
	// Output:
	// deps.depends_on(app, util)
	//   deps.depends_on(app, log)
	//     deps.depends_on(app, api)
	//       imports(app, api)  imports.txt:1
	//     imports(api, log)  imports.txt:3
	//   imports(log, util)  imports.txt:5
}

func show(w *datalog.Witness, depth int) {
	vals := make([]string, len(w.Values))
	for i, v := range w.Values {
		vals[i] = v.S
	}
	line := strings.Repeat("  ", depth) + w.Relation + "(" + strings.Join(vals, ", ") + ")"
	if len(w.Cites) > 0 {
		line += "  " + strings.Join(w.Cites, ", ")
	}
	fmt.Println(line)
	for _, c := range w.Children {
		show(c, depth+1)
	}
}
