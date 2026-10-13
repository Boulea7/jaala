package datalog

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panyam/jaala/ns"
)

// digits is n(1) to n(10), so n(?a), n(?b), n(?c) finds a thousand bindings.
func digits() *ns.MemSource {
	src := ns.NewMemSource().DeclareSchema("n", ns.Schema{Arity: 1, Labels: []string{"v"}, Types: []ns.ArgType{{Type: ns.TypeNumber}}})
	for i := 1; i <= 10; i++ {
		src.Add("n", ns.Tuple{Vals: []ns.Value{ns.N(float64(i))}, Cites: []string{fmt.Sprintf("n%d", i)}})
	}
	return src
}

// reported runs q under the planned SemiNaive with Explain, for what the Eval held and the work it did.
func reported(t *testing.T, src ns.Source, q string, opts ...Option) ([]Row, Report) {
	t.Helper()
	var r Report
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, q), baseFor(std(src)), append(opts, Explain(&r))...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return rows, r
}

const cube = `n(?a), n(?b), n(?c)`

func TestTheGoalHoldsItsAnswerNotItsBindings(t *testing.T) {
	rows, r := reported(t, digits(), cube+` => count(?a)`)
	if got := col(rows, "count(a)"); got != "1000" {
		t.Fatalf("control: count = %s, want 1000 bindings", got)
	}
	if r.Held != 1 {
		t.Errorf("a count over 1000 bindings held %d, want its one group", r.Held)
	}
	rows, r = reported(t, digits(), cube+` => ?a`)
	if len(rows) != 10 || r.Held != 10 {
		t.Errorf("a projection of 1000 bindings onto 10 rows: %d rows, held %d, want 10 and 10", len(rows), r.Held)
	}
	if rows, r = reported(t, digits(), cube+` => ?a, ?b, ?c`); len(rows) != 1000 || r.Held != 1000 {
		t.Errorf("control: every binding a distinct row: %d rows, held %d, want 1000 and 1000", len(rows), r.Held)
	}
}

func TestASumIsExactWhateverItsOrder(t *testing.T) {
	if a, b, c := 0.1, 0.2, 0.3; a+b+c == 0.6 {
		t.Fatal("control: adding 0.1, 0.2 and 0.3 in turn is already 0.6, so this fixture can't tell an exact sum")
	}
	src := ns.NewMemSource().Declare("x", "k", "v")
	for i, v := range []float64{0.1, 0.2, 0.3, 1e100, 1, -1e100} {
		src.Add("x", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprint(i)), ns.N(v)}})
	}
	for _, c := range []struct{ q, col, want string }{
		{`x(?k, ?v), ?v > 0, ?v < 1 => sum(?v)`, "sum(v)", "0.6"},
		{`x(?k, ?v), ?v != 0.1, ?v != 0.2, ?v != 0.3 => sum(?v)`, "sum(v)", "1"}, // 1e100 + 1 - 1e100
		{`agg(sum(?v)) :- x(_, ?v), ?v > 0, ?v < 1; agg(?s) => ?s`, "s", "0.6"},
	} {
		if got := col(eval(t, src, c.q), c.col); got != c.want {
			t.Errorf("%s = %s, want %s", c.q, got, c.want)
		}
	}
}

func TestALimitWithoutAnOrderStopsOnceItHasItsRows(t *testing.T) {
	_, whole := reported(t, digits(), cube+` => ?a, ?b, ?c`)
	rows, r := reported(t, digits(), cube+` => ?a, ?b, ?c limit 5`)
	if len(rows) != 5 || r.Held != 5 {
		t.Fatalf("limit 5: %d rows, held %d, want 5 and 5", len(rows), r.Held)
	}
	if r.Work*10 > whole.Work {
		t.Errorf("limit 5 did %d work, the whole answer %d: want it to stop early", r.Work, whole.Work)
	}
	both(mustParse(t, cube+` => ?a, ?b, ?c limit 5`), baseFor(std(digits()))) // five rows of the whole answer in each evaluator
	var n int
	if rows, r := reported(t, digits(), cube+` => ?a, ?b, ?c limit 5`, Total(&n)); len(rows) != 5 || n != 1000 || r.Work < whole.Work {
		t.Errorf("limit 5 with Total: %d rows, total %d, work %d of %d; want 5, 1000, and the whole goal solved", len(rows), n, r.Work, whole.Work)
	}
}

func TestAnOrderedLimitHoldsOnlyItsPage(t *testing.T) {
	q := cube + ` => ?a, ?b, ?c order by ?c desc, ?b, ?a`
	whole := cols(eval(t, digits(), q), "a", "b", "c")
	rows, r := reported(t, digits(), q+` limit 5 offset 3`)
	if got := cols(rows, "a", "b", "c"); !reflect.DeepEqual(got, whole[3:8]) {
		t.Errorf("limit 5 offset 3 = %v, want %v", got, whole[3:8])
	}
	if r.Held > 8 {
		t.Errorf("limit 5 offset 3 held %d rows, want at most 8", r.Held)
	}
	if got := cols(eval(t, digits(), q+` limit 5 offset 3`), "a", "b", "c"); !reflect.DeepEqual(got, whole[3:8]) {
		t.Errorf("every evaluator: %v, want %v", got, whole[3:8])
	}
}

func TestPagesAfterTheLastRowAreTheWholeAnswer(t *testing.T) {
	q := `part(?r, ?n) => ?r, ?n order by ?n desc, ?r`
	whole := eval(t, parts(), q)
	if len(whole) != 13 {
		t.Fatalf("control: %d rows, want 13", len(whole))
	}
	for size := 1; size <= 14; size++ {
		var pages []Row
		var opts []Option
		for n := 0; ; n++ {
			if n > len(whole) {
				t.Fatalf("pages of %d: still paging after %d pages: after isn't moving on", size, n)
			}
			page := evalOpts(t, parts(), fmt.Sprintf("%s limit %d", q, size), opts...)
			pages = append(pages, page...)
			if len(page) < size {
				break
			}
			opts = []Option{After(page[len(page)-1].Bind)}
		}
		if !reflect.DeepEqual(binds(pages), binds(whole)) {
			t.Errorf("pages of %d: %v, want %v", size, cols(pages, "r", "n"), cols(whole, "r", "n"))
		}
	}
	// The clause says the same in text, and a cursor needn't be a row the answer still has.
	got := cols(eval(t, parts(), q+` after "GND", "R0" limit 2`), "r", "n")
	if want := cols(whole, "r", "n"); !reflect.DeepEqual(got, want[indexOf(want, "R1 GND"):][:2]) {
		t.Errorf("after GND, R0 = %v, want the first two GND rows %v", got, want[indexOf(want, "R1 GND"):][:2])
	}
}

func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return -1
}

func evalOpts(t *testing.T, src ns.Source, q string, opts ...Option) []Row {
	t.Helper()
	rows, err := both(mustParse(t, q), baseFor(std(src)), opts...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return rows
}

func TestTotalCountsTheRowsBeforeThePage(t *testing.T) {
	for q, want := range map[string]int{
		`part(?r, ?n) => ?r, ?n order by ?n desc, ?r limit 2`:                   13,
		`part(?r, ?n) => ?r, ?n order by ?n desc, ?r after "GND", "R0" limit 2`: 13,
		`part(?r, ?n) => ?n, count(?r) order by ?n limit 1`:                     4,
		`part(?r, ?n) => ?n limit 1`:                                            4,
	} {
		var n int
		evalOpts(t, parts(), q, Total(&n))
		if n != want {
			t.Errorf("%s: Total %d, want %d", q, n, want)
		}
	}
}

func TestEvalTableHoldsTheAnswerAsColumns(t *testing.T) {
	b := baseFor(std(parts()))
	tab, err := SemiNaive{}.EvalTable(bg, mustParse(t, `part(?r, ?n), ?n = "VBUS" => ?n, count(?r), min(?r) order by ?n`), b)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Var{"n", "count(r)", "min(r)"}; !reflect.DeepEqual(tab.Columns, want) {
		t.Errorf("columns %v, want %v", tab.Columns, want)
	}
	if tab.Len() != 1 || tab.Rows[0][0].S != "VBUS" || tab.Rows[0][1].S != "2" || !tab.Rows[0][2].Absent {
		t.Fatalf("rows %+v, want VBUS, 2, absent", tab.Rows)
	}
	if tab.Why == nil || tab.Why[0][2] != "no numbers" || tab.Why[0][1] != "" {
		t.Errorf("why %q, want only min(r)'s reason", tab.Why)
	}
	if tab.Witness != nil {
		t.Errorf("witness %v without Witnesses(), want nil", tab.Witness)
	}
	tab, err = SemiNaive{}.EvalTable(bg, mustParse(t, `part(?r, "SDA") => ?r`), b, Witnesses())
	if err != nil || tab.Len() != 1 || tab.Rows[0][0].S != "R0" || len(tab.Witness) != 1 || len(tab.Witness[0]) != 1 {
		t.Errorf("witnessed table %+v, %v; want R0 with one witness node", tab, err)
	}
	if row := tab.Row(0); row.Bind["r"].S != "R0" || len(row.Witness) != 1 || row.Why != nil {
		t.Errorf("Row(0) = %+v, want R0 with its witness and no reasons", row)
	}
}

func TestPagingRefusals(t *testing.T) {
	for _, c := range []struct {
		q    string
		opts []Option
		want string
	}{
		{`part(?r, ?n) => ?r offset 2`, nil, `offset needs an order by`},
		{`part(?r, ?n) => ?r, count(?n) offset 1`, nil, `offset needs an order by`},
		{`part(?r, ?n) => ?r after "R1"`, nil, `after needs an order by`},
		{`part(?r, ?n) => ?r, ?n order by ?n after "GND", "R1"`, nil, `after gives 2 values for 1 order by columns`},
		{`part(?r, ?n) => ?r, ?n order by ?n after "GND"`, nil, `after needs an order by naming ?r too`},
		{`part(?r, ?n) => ?r order by ?r after ?n`, nil, `after takes values`},
		{`part(?r, ?n) => ?r order by ?r limit 1 after "R1"`, nil, `limit comes after after, not before it (the order is having, order by, after, limit, offset)`},
		{`part(?r, ?n) => ?r, ?n order by ?n, ?r limit 1`, []Option{After(map[Var]ns.Value{"n": ns.S("GND")})}, `After's row has no ?r, a column the order by reads`},
		{`part(?r, ?n) => ?r limit 1`, []Option{After(map[Var]ns.Value{"r": ns.S("R1")})}, `after needs an order by`},
	} {
		q, err := Parse(c.q)
		if err == nil {
			_, err = both(q, baseFor(std(parts())), c.opts...)
		}
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.HasPrefix(err.Error(), "query: ") {
			t.Errorf("%s: err = %v, want it to say %q", c.q, err, c.want)
		}
	}
}

// BenchmarkMemoryOfACountOverACrossProduct is #120's repro: a count over a three-way join of a
// thousand-tuple cross product, stopped by its budget. It reports the peak heap in use above where it
// started, which grew with the bindings when the goal held them all (3.5 GB at a budget of 2,000,000,
// where wasm ran out of memory) and is now the answer's size.
func BenchmarkMemoryOfACountOverACrossProduct(b *testing.B) {
	for _, budget := range []int64{100_000, 500_000} {
		b.Run(fmt.Sprint(budget), func(b *testing.B) {
			q := MustParse(`p(?a, ?b, ?c) :- n(?a), n(?b), n(?c); p(?a, ?b, ?c), p(?d, ?e, ?f), p(?g, ?h, ?i) => count(?a)`)
			base := MustBase(std(digits()), digits())
			base.LimitDerivedCache(0) // every run derives p, as a first query does
			b.ReportAllocs()
			var peak uint64
			for i := 0; i < b.N; i++ {
				var err error
				peak = max(peak, peakHeapDuring(func() { _, err = SemiNaive{}.Eval(bg, q, base, Budget(budget)) }))
				var over *BudgetExceeded
				if err != nil && !errors.As(err, &over) {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(peak)/1e6, "peak-MB")
		})
	}
}

// peakHeapDuring runs f and returns the peak heap in use above where it started, sampled every
// millisecond.
func peakHeapDuring(f func()) uint64 {
	runtime.GC()
	var start runtime.MemStats
	runtime.ReadMemStats(&start)
	stop := make(chan struct{})
	sampled := make(chan uint64)
	go func() {
		var ms runtime.MemStats
		var top uint64
		for {
			runtime.ReadMemStats(&ms)
			top = max(top, ms.HeapInuse)
			select {
			case <-stop:
				sampled <- top
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	f()
	close(stop)
	if top := <-sampled; top > start.HeapInuse {
		return top - start.HeapInuse
	}
	return 0
}
