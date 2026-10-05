package datalog

import (
	"fmt"
	"testing"

	"github.com/panyam/jaala/ns"
)

// probedBoard is n decoupling capacitors, each between GND and its own net N<i>, with a test point on
// every N<i> and n more on GND: the skew of a real board (#139), where one wide net carries most of
// the two-terminal parts and many of the test points.
func probedBoard(n int) *ns.MemSource {
	src := ns.NewMemSource().Declare("part", "ref", "class").Declare("pin", "ref", "net")
	add := func(ref, class string, nets ...string) {
		src.Add("part", ns.Tuple{Vals: []ns.Value{ns.S(ref), ns.S(class)}})
		for _, net := range nets {
			src.Add("pin", ns.Tuple{Vals: []ns.Value{ns.S(ref), ns.S(net)}})
		}
	}
	for i := 0; i < n; i++ {
		add(fmt.Sprintf("C%d", i), "capacitor", "GND", fmt.Sprintf("N%d", i))
		add(fmt.Sprintf("TP%d", i), "test_point", fmt.Sprintf("N%d", i))
		add(fmt.Sprintf("TG%d", i), "test_point", "GND")
	}
	return src
}

const probed = `has_tp(?n) :- pin(?tp, ?n), part(?tp, "test_point"); ` +
	`tt(?r, ?a, ?b) :- part(?r, "capacitor"), pin(?r, ?a), pin(?r, ?b), ?a < ?b; ` +
	`probed_both(?r) :- tt(?r, ?a, ?b), has_tp(?a), has_tp(?b); `

// workAt answers query over probedBoard(n) under the planned SemiNaive, checking that every capacitor
// is counted, and returns the work it took.
func workAt(t *testing.T, query string, n int) int64 {
	t.Helper()
	b := baseFor(std(probedBoard(n)))
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, query), b)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if len(rows) != 1 || len(rows[0].Bind) != 1 || col(rows, singleColumn(rows[0])) != fmt.Sprint(n) {
		t.Fatalf("%s over %d: rows = %v, want a count of %d", query, n, rows, n)
	}
	return b.Work()
}

// Probing a small derived relation one value at a time costs a lookup per value, not its whole body
// (#139), whatever order the goal is written in: doubling the board must about double the work.
// control: probing the relation's body by hand does list every test point on GND per capacitor, so
// the board can tell a probe from a re-run body.
func TestProbingADerivedRelationStaysLinear(t *testing.T) {
	for _, c := range []struct{ name, query string }{
		{"the rule", probed + `probed_both(?r) => count(?r)`},
		{"the goal as written", probed + `tt(?r, ?a, ?b), has_tp(?a), has_tp(?b) => count(?r)`},
		{"the goal reordered", probed + `has_tp(?a), tt(?r, ?a, ?b), has_tp(?b) => count(?r)`},
	} {
		small, large := workAt(t, c.query, 200), workAt(t, c.query, 400)
		if float64(large) > 2.3*float64(small) {
			t.Errorf("%s: work %d at 200 parts, %d at 400 (x%.1f), want about double", c.name, small, large, float64(large)/float64(small))
		}
	}
	byHand := `part(?r, "capacitor"), pin(?r, ?a), pin(?r, ?b), ?a < ?b, ` +
		`pin(?t, ?a), part(?t, "test_point"), pin(?u, ?b), part(?u, "test_point") => count(distinct ?r)`
	if small, large := workAt(t, byHand, 200), workAt(t, byHand, 400); float64(large) < 3*float64(small) {
		t.Errorf("control: the body by hand took %d then %d, want it quadratic", small, large)
	}
}

// singleColumn is the name of a one-column row's only binding.
func singleColumn(r Row) string {
	for k := range r.Bind {
		return string(k)
	}
	return ""
}
