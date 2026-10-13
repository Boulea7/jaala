package datalog

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"

	"github.com/panyam/jaala/ns"
)

// A wide is one relation edge(a, k, b) of n facts with no self-loops, made as they're read, so a
// read that stops early never makes the rest. It counts the facts it made.
type wide struct {
	n    int
	made int
}

func (w *wide) Schema(rel string) (ns.Schema, bool) {
	return ns.Schema{Arity: 3, Labels: []string{"a", "k", "b"}}, rel == "edge"
}

func (w *wide) Relations() []string { return []string{"edge"} }

func (w *wide) fact(i int) ns.Tuple {
	w.made++
	return ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("n%d", i)), ns.S("calls"), ns.S(fmt.Sprintf("n%d", i+1))}}
}

// Tuples makes every fact, as a Source returning one slice does.
func (w *wide) Tuples(string) []ns.Tuple {
	out := make([]ns.Tuple, w.n)
	for i := range out {
		out[i] = w.fact(i)
	}
	return out
}

// A streamingWide is a wide that streams.
type streamingWide struct{ *wide }

func (w streamingWide) TuplesSeq(_ context.Context, _ string) iter.Seq2[ns.Tuple, error] {
	return func(yield func(ns.Tuple, error) bool) {
		for i := range w.n {
			if !yield(w.fact(i), nil) {
				return
			}
		}
	}
}

// selfLoops is #233's question: an unbound call, so the relation is read whole.
const selfLoops = `edge(?a, ?k, ?a) => ?a, ?k`

// A relation a Source streams is charged to the budget as it's read, so a query over one larger than
// its budget stops having made and held at most that many facts, in every evaluator (#233). A
// Source returning one slice makes them all first.
func TestAWholeReadStopsAtTheBudget(t *testing.T) {
	const n, budget = 200_000, 1_000
	q := mustParse(t, selfLoops)
	for _, ev := range []Evaluator{Naive{}, SemiNaive{WrittenOrder: true}, SemiNaive{}} {
		w := &wide{n: n}
		var err error
		peak := peakHeapDuring(func() {
			_, err = ev.Eval(bg, q, MustBase(ns.MustVocabulary(streamingWide{w}), streamingWide{w}), Budget(budget))
		})
		var over *BudgetExceeded
		if !errors.As(err, &over) {
			t.Fatalf("%T: %v, want the budget exceeded", ev, err)
		}
		if w.made > budget+1 {
			t.Errorf("%T made %d facts under a budget of %d, want the read stopped at the budget", ev, w.made, budget)
		}

		slice := &wide{n: n}
		var sliceErr error
		slicePeak := peakHeapDuring(func() { _, sliceErr = ev.Eval(bg, q, MustBase(ns.MustVocabulary(slice), slice), Budget(budget)) })
		if !errors.As(sliceErr, &over) || slice.made != n {
			t.Errorf("control: %T over a slice made %d facts and answered %v, want all %d and the budget exceeded", ev, slice.made, sliceErr, n)
		}
		if peak*10 > slicePeak {
			t.Errorf("%T held %d bytes streaming and %d reading a slice, want a tenth or less", ev, peak, slicePeak)
		}
	}
}

// A read the budget stopped isn't kept: the next query on the Base reads the relation again, whole,
// and answers. A streamed read isn't work either, so Work and Explain see what reading a slice does.
func TestAReadStoppedAtTheBudgetIsNotKept(t *testing.T) {
	const n = 5_000
	q := mustParse(t, selfLoops)
	w := &wide{n: n}
	b := MustBase(ns.MustVocabulary(streamingWide{w}), streamingWide{w})
	if _, err := (SemiNaive{}).Eval(bg, q, b, Budget(100)); err == nil {
		t.Fatal("control: a budget of 100 answered a read of 5000 facts")
	}
	w.made = 0
	var r Report
	rows, err := SemiNaive{}.Eval(bg, q, b, Explain(&r))
	if err != nil || len(rows) != 0 || w.made != n {
		t.Fatalf("after a stopped read: %d rows, %v, made %d facts; want none, no error, all %d read again", len(rows), err, w.made, n)
	}

	var sliced Report
	slice := &wide{n: n}
	if _, err := (SemiNaive{}).Eval(bg, q, MustBase(ns.MustVocabulary(slice), slice), Explain(&sliced)); err != nil {
		t.Fatal(err)
	}
	if r.Work != sliced.Work || b.Work() != sliced.Work {
		t.Errorf("a streamed read reported work %d (Base %d), a slice %d: reading isn't work", r.Work, b.Work(), sliced.Work)
	}
}
