package architecture_test

import (
	"context"
	"fmt"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
)

// chain is a line of imports, p000 -> p001 -> ... -> p199.
func chain() *ns.MemSource {
	src := ns.NewMemSource().Declare("imports", "from", "to")
	for i := 0; i < 199; i++ {
		src.Add("imports", ns.Tuple{Vals: []ns.Value{
			ns.S(fmt.Sprintf("p%03d", i)), ns.S(fmt.Sprintf("p%03d", i+1)),
		}})
	}
	return src
}

const dependsOn = `
depends_on(?a, ?b) :- imports(?a, ?b);
depends_on(?a, ?c) :- depends_on(?a, ?b), imports(?b, ?c);
`

// The demand rewrite derives only what a query's constants ask for. Asked
// about one package near the end of the line, the planned SemiNaive walks
// from it, where the written order derives every pair first.
func Example_demand() {
	src := chain()
	v, _ := ns.NewVocabulary(src)
	q := datalog.MustParse(dependsOn + `depends_on("p190", ?d) => ?d`)
	work := func(ev datalog.Evaluator) (int, int64) {
		b, _ := datalog.NewBase(v, src)
		rows, err := ev.Eval(context.Background(), q, b)
		if err != nil {
			panic(err)
		}
		return len(rows), b.Work()
	}
	n1, written := work(datalog.SemiNaive{WrittenOrder: true})
	n2, planned := work(datalog.SemiNaive{})
	fmt.Println(n1, "rows written,", n2, "rows planned")
	fmt.Println("planned does under 1% of the work:", planned*100 < written)
	// Output:
	// 9 rows written, 9 rows planned
	// planned does under 1% of the work: true
}

// Rules are checked as written, before any rewrite renames them, so an
// error names the relation you wrote, whatever the planner made of it.
func Example_errorsNameWhatYouWrote() {
	src := chain()
	v, _ := ns.NewVocabulary(src)
	b, _ := datalog.NewBase(v, src)
	q := datalog.MustParse(dependsOn + `depends_on("p190", ?d, ?e) => ?d`)
	_, err := datalog.SemiNaive{}.Eval(context.Background(), q, b)
	fmt.Println(err)
	// Output:
	// query: relation "depends_on" takes 2 args, got 3
}
