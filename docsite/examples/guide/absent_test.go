package guide_test

import (
	"context"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// An absent value is a field the source didn't state. It sorts before
// everything else, it isn't the same as "", and absent(?x) asks about it.
func Example_absent() {
	b, ctx := base(), context.Background()
	for _, text := range []string{
		`pkg(?p, ?l) => ?l, ?p`,
		`pkg(?p, ?l), absent(?l) => ?p`,
		`pkg(?p, ?l), not absent(?l) => ?p`,
		`pkg(?p, "") => ?p`,
	} {
		rows, err := datalog.SemiNaive{}.Eval(ctx, datalog.MustParse(text), b)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println(text)
		for _, r := range rows {
			if l, ok := r.Bind["l"]; ok {
				fmt.Println("  ", r.Bind["p"].S, show(l))
			} else {
				fmt.Println("  ", r.Bind["p"].S)
			}
		}
	}
	// Output:
	// pkg(?p, ?l) => ?l, ?p
	//    api absent
	//    log ""
	//    util "Apache-2.0"
	//    app "MIT"
	// pkg(?p, ?l), absent(?l) => ?p
	//    api
	// pkg(?p, ?l), not absent(?l) => ?p
	//    app
	//    log
	//    util
	// pkg(?p, "") => ?p
	//    log
}

// show prints a value as a host might: quoted text, or "absent".
func show(v ns.Value) string {
	if v.Absent {
		return "absent"
	}
	return fmt.Sprintf("%q", v.S)
}
