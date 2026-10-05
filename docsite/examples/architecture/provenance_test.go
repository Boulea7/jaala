package architecture_test

import (
	"context"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// A row cites the facts of the derivation that found it first. When a
// tuple can be derived two ways, the evaluators can find different ones
// first, and cite different, equally valid evidence (#22).
func Example_firstDerivation() {
	src := ns.NewMemSource().Declare("imports", "from", "to")
	for _, e := range [][2]string{{"v2", "v1"}, {"v3", "v2"}} {
		src.Add("imports", ns.Tuple{
			Vals:  []ns.Value{ns.S(e[0]), ns.S(e[1])},
			Cites: []string{e[0] + "->" + e[1]},
		})
	}
	v, _ := ns.NewVocabulary(src)
	// pair(?p, ?p) holds two ways: for a package that imports something
	// (through sender, one rule deeper), and for one something imports.
	// Within a round Naive takes relations in name order, and sender sorts
	// after pair, so Naive finds v2 through the import first. SemiNaive
	// derives sender before the rules that read it, so it finds the other.
	q := datalog.MustParse(`
		pair(?a, ?a) :- sender(?a);
		pair(?b, ?b) :- imports(?c, ?b);
		sender(?d) :- imports(?d, ?e);
		pair(?x, ?y) => ?y`)
	for _, ev := range []datalog.Evaluator{
		datalog.Naive{}, datalog.SemiNaive{WrittenOrder: true},
	} {
		b, _ := datalog.NewBase(v, src)
		rows, err := ev.Eval(context.Background(), q, b)
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
	// datalog.SemiNaive cites [v2->v1] for v2
}
