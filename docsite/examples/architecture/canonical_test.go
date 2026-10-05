package architecture_test

import (
	"context"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// CanonicalCites makes every evaluator keep the same derivation: the
// shortest, here the one step from the import into v2. Ties between
// equally short ones go to the rule written first by its text, then to
// the body facts that come first by value.
func Example_canonicalCites() {
	src := ns.NewMemSource().Declare("imports", "from", "to")
	for _, e := range [][2]string{{"v2", "v1"}, {"v3", "v2"}} {
		src.Add("imports", ns.Tuple{
			Vals:  []ns.Value{ns.S(e[0]), ns.S(e[1])},
			Cites: []string{e[0] + "->" + e[1]},
		})
	}
	v, _ := ns.NewVocabulary(src)
	q := datalog.MustParse(`
		pair(?a, ?a) :- sender(?a);
		pair(?b, ?b) :- imports(?c, ?b);
		sender(?d) :- imports(?d, ?e);
		pair(?x, ?y) => ?y`)
	for _, ev := range []datalog.Evaluator{
		datalog.Naive{}, datalog.SemiNaive{WrittenOrder: true}, datalog.SemiNaive{},
	} {
		b, _ := datalog.NewBase(v, src)
		rows, err := ev.Eval(context.Background(), q, b, datalog.CanonicalCites())
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, r := range rows {
			if r.Bind["y"].S == "v2" {
				fmt.Printf("%T cites %v for v2\n", ev, r.Cites)
			}
		}
	}
	// Output:
	// datalog.Naive cites [v3->v2] for v2
	// datalog.SemiNaive cites [v3->v2] for v2
	// datalog.SemiNaive cites [v3->v2] for v2
}
