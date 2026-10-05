package reference_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// The three ways to evaluate a query answer it the same way. They differ
// in the work they do: Naive repeats every rule each round, SemiNaive in
// written order only revisits what changed, and the planned SemiNaive also
// derives only what the query's constants ask for and plans each body.
func Example_evaluators() {
	src := ns.NewMemSource().Declare("imports", "from", "to")
	for i := 0; i < 30; i++ {
		src.Add("imports", ns.Tuple{Vals: []ns.Value{
			ns.S(fmt.Sprintf("p%02d", i)), ns.S(fmt.Sprintf("p%02d", i+1)),
		}})
	}
	v, err := ns.NewVocabulary(src)
	if err != nil {
		fmt.Println(err)
		return
	}
	q := datalog.MustParse(`
		depends_on(?a, ?b) :- imports(?a, ?b);
		depends_on(?a, ?c) :- depends_on(?a, ?b), imports(?b, ?c);
		depends_on("p25", ?d) => ?d`)

	var answers [][]datalog.Row
	var work []int64
	for _, ev := range []datalog.Evaluator{
		datalog.Naive{},
		datalog.SemiNaive{WrittenOrder: true},
		datalog.SemiNaive{},
	} {
		b, _ := datalog.NewBase(v, src)
		rows, err := ev.Eval(context.Background(), q, b)
		if err != nil {
			fmt.Println(err)
			return
		}
		answers = append(answers, rows)
		work = append(work, b.Work())
	}
	fmt.Println(len(answers[0]), "rows from each")
	fmt.Println("same answers:", reflect.DeepEqual(answers[0], answers[1]) &&
		reflect.DeepEqual(answers[1], answers[2]))
	fmt.Println("less work each step:", work[0] > work[1] && work[1] > work[2])
	// Output:
	// 5 rows from each
	// same answers: true
	// less work each step: true
}
