package datalog

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/panyam/jaala/ns"
)

// counting is a Source that counts the relations read whole.
type counting struct {
	ns.Source
	mu     sync.Mutex
	wholes map[string]int
}

func (c *counting) Tuples(rel string) []ns.Tuple {
	c.mu.Lock()
	c.wholes[rel]++
	c.mu.Unlock()
	return c.Source.Tuples(rel)
}

func (c *counting) whole(rel string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wholes[rel]
}

// looking is a counting Source that also answers Lookup, by scanning with the engine's equality over
// each value read as its argument's declared type, as the LookupSource contract asks. sloppy
// returns the whole relation from every lookup, which the contract allows; fail makes every lookup
// fail with it.
type looking struct {
	*counting
	sloppy  bool
	fail    error
	lookups map[string]int
}

func lookingAt(src ns.Source) *looking {
	return &looking{counting: &counting{Source: src, wholes: map[string]int{}}, lookups: map[string]int{}}
}

func (l *looking) Lookup(ctx context.Context, rel string, bound map[int]ns.Value) ([]ns.Tuple, error) {
	l.mu.Lock()
	l.lookups[rel]++
	fail := l.fail
	l.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	all := l.Source.Tuples(rel)
	if l.sloppy {
		return all, nil
	}
	s, _ := l.Source.Schema(rel)
	read := normalizeTuples(all, s.Types)
	var out []ns.Tuple
	for k, t := range all {
		ok := true
		for i, v := range bound {
			ok = ok && valueEq(read[k].Vals[i], v)
		}
		if ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (l *looking) looked(rel string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lookups[rel]
}

var lookupQueries = []string{
	`edge("a", ?y) => ?y`,
	`edge(?x, ?y), edge(?y, ?z) => ?x, ?z`,
	closure + `reach("a", ?b) => ?b`,
	closure + `reach(?a, ?b), not edge(?a, ?b) => ?a, ?b`,
	closure + `node(?n), reach(?n, "d") => count(?n)`,
	`weight(?n, ?w), ?w > 2, edge(?n, ?m) => ?n, ?m`,
	`two(?a, ?c) :- edge(?a, ?b), edge(?b, ?c); two(?a, ?c) :- edge(?a, ?c), node(?c); node(?n), two(?n, ?c) => ?n, ?c`,
}

// A Source answering lookups gives every query the rows, citations and witnesses reading it whole
// does, in all three evaluators, and so does one whose lookups return more than matches (#126).
// control: the lookups are made.
func TestALookupSourceAnswersAsReadingItWhole(t *testing.T) {
	v := std(graph())
	for _, sloppy := range []bool{false, true} {
		src := lookingAt(graph())
		src.sloppy = sloppy
		b := MustBase(v, src)
		for _, text := range lookupQueries {
			q := mustParse(t, text)
			want, werr := both(q, baseFor(v))
			got, gerr := both(q, b)
			if !reflect.DeepEqual(binds(got), binds(want)) || errText(gerr) != errText(werr) {
				t.Errorf("sloppy %v: %s\n looked up: %v %v\n whole:     %v %v", sloppy, text, binds(got), gerr, binds(want), werr)
			}
			opts := []Option{CanonicalCites(), Witnesses()}
			if strings.Contains(text, "?y") {
				opts = append(opts, Bind(map[Var][]ns.Value{"y": {ns.S("b"), ns.S("c")}}))
			}
			for i, opt := range opts {
				w, werr := Naive{}.Eval(bg, q, baseFor(v), opt)
				g, gerr := Naive{}.Eval(bg, q, b, opt)
				if !reflect.DeepEqual(g, w) || errText(gerr) != errText(werr) {
					t.Errorf("sloppy %v: %s with option %d\n looked up: %v %v\n whole:     %v %v", sloppy, text, i, g, gerr, w, werr)
				}
			}
		}
		if src.looked("edge") == 0 {
			t.Errorf("control: sloppy %v made no lookups", sloppy)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// A call with an argument bound is looked up, never read whole, in every evaluator, the planned
// SemiNaive's estimates (fanOut, and a delta round's scanSize) included. Under demand that covers rule
// bodies too, which Naive and the written order derive in full, reading what they call whole.
// control: the same queries over a Source without Lookup read edge whole.
func TestABoundCallNeverReadsItsRelationWhole(t *testing.T) {
	v := std(reversedLine(40))
	cases := []struct {
		text string
		evs  []Evaluator
	}{
		{`edge("v3", ?y), edge(?y, ?z) => ?z`, []Evaluator{Naive{}, SemiNaive{WrittenOrder: true}, SemiNaive{}}},
		{closure + `reach("v3", ?b) => count(?b)`, []Evaluator{SemiNaive{}}},
		{`two(?a, ?c) :- edge(?a, ?b), edge(?b, ?c); two(?a, ?c) :- edge(?a, ?c), edge(?c, "v9"); two("v7", ?c) => ?c`, []Evaluator{SemiNaive{}}},
		// Evaluated in full, its recursive rule planned to start from edge("v1", ?y), which a round sizes
		// against its delta.
		{`lab(?y) :- edge("v3", ?y); lab(?z) :- edge("v1", ?y), lab(?x), edge(?x, ?z); lab(?y) => ?y`, []Evaluator{SemiNaive{}}},
	}
	for _, c := range cases {
		for _, ev := range c.evs {
			src := lookingAt(reversedLine(40))
			if _, err := ev.Eval(bg, mustParse(t, c.text), MustBase(v, src)); err != nil {
				t.Fatal(err)
			}
			if src.whole("edge") != 0 || src.looked("edge") == 0 {
				t.Errorf("%T: %s read edge whole %d times, with %d lookups; want none, and lookups", ev, c.text, src.whole("edge"), src.looked("edge"))
			}
			plain := &counting{Source: reversedLine(40), wholes: map[string]int{}}
			if _, err := ev.Eval(bg, mustParse(t, c.text), MustBase(v, plain)); err != nil || plain.whole("edge") == 0 {
				t.Errorf("control: %T: %s over a plain source read edge whole %d times (%v)", ev, c.text, plain.whole("edge"), err)
			}
		}
	}
}

// A call with nothing bound reads the relation whole, and the Base then answers bound calls from its
// own index, in that Eval and later ones, rather than asking the Source. Unindexed never looks up.
func TestAWholeReadAnswersLaterCalls(t *testing.T) {
	v := std(reversedLine(40))
	src := lookingAt(reversedLine(40))
	b := MustBase(v, src)
	if _, err := (Naive{}).Eval(bg, mustParse(t, `edge("v1", ?y) => ?y`), b); err != nil {
		t.Fatal(err)
	}
	if src.looked("edge") != 1 || src.whole("edge") != 0 {
		t.Fatalf("control: a bound call made %d lookups and %d whole reads; want one lookup", src.looked("edge"), src.whole("edge"))
	}
	if _, err := (Naive{}).Eval(bg, mustParse(t, `edge(?x, ?y), edge(?y, ?z) => ?x, ?z`), b); err != nil {
		t.Fatal(err)
	}
	if _, err := (Naive{}).Eval(bg, mustParse(t, `edge("v2", ?y) => ?y`), b); err != nil {
		t.Fatal(err)
	}
	if src.looked("edge") != 1 || src.whole("edge") != 1 {
		t.Errorf("after a whole read: %d lookups, %d whole reads; want the one lookup from before, and one read", src.looked("edge"), src.whole("edge"))
	}
	un := lookingAt(reversedLine(40))
	if _, err := (SemiNaive{}).Eval(bg, mustParse(t, `edge("v1", ?y) => ?y`), MustBase(v, un).Unindexed()); err != nil {
		t.Fatal(err)
	}
	if un.looked("edge") != 0 || un.whole("edge") != 1 {
		t.Errorf("Unindexed: %d lookups, %d whole reads; want it to read the relation, as the oracle", un.looked("edge"), un.whole("edge"))
	}
}

// What a lookup returns is kept for its Eval: Naive, in written order, calls edge("a", ?y) once per
// node, and asks the Source once. The next Eval asks again, so the Base never holds the relation.
// Each lookup counts one unit of work, and a call answered again counts none.
func TestALookupIsKeptForItsEvalOnly(t *testing.T) {
	v := std(graph())
	src := lookingAt(graph())
	b := MustBase(v, src)
	q := `node(?n), edge("a", ?y) => ?n, ?y`
	r := explained(t, Naive{}, q, b)
	if src.looked("edge") != 1 {
		t.Errorf("one Eval made %d lookups of edge(\"a\", ?y); want 1", src.looked("edge"))
	}
	edge := sourceIn(r, "edge")
	if edge == nil || edge.Lookups != 1 || edge.Hits != 4 || edge.Fetched != 1 || edge.Tuples != 0 {
		t.Errorf("edge's report: %+v; want 1 lookup fetching 1 tuple, answered again 4 times", edge)
	}
	if r.Lookups != 1 || r.Fetched != 1+5 {
		t.Errorf("report totals: %d lookups, %d fetched; want 1, and edge's tuple with node's 5", r.Lookups, r.Fetched)
	}
	// node is scanned (5) and each of the 5 calls reads edge's one tuple, plus the one lookup.
	if r.Work != 5+5+1 {
		t.Errorf("work %d; want 11: node's 5, edge's tuple 5 times, one lookup", r.Work)
	}
	if lr := r.Goal.Literals[1]; fmt_(lr.Access) != "lookup (from)" {
		t.Errorf("edge's access: %q", lr.Access)
	}
	if !strings.Contains(r.String(), "edge: looked up, 1 lookups fetching 1 tuples, 4 answered from earlier lookups") {
		t.Errorf("text:\n%s", r)
	}
	explained(t, Naive{}, q, b)
	if src.looked("edge") != 2 || b.edb.holds("edge") {
		t.Errorf("a second Eval: %d lookups in all, edge held %v; want 2, not held", src.looked("edge"), b.edb.holds("edge"))
	}
}

func sourceIn(r *Report, rel string) *SourceReport {
	for _, s := range r.Sources {
		if s.Relation == rel {
			return s
		}
	}
	return nil
}

// A failed lookup stops the query that asked with a query: error naming the relation, and isn't kept:
// the next Eval looks up again and answers. A cancelled one says the evaluation stopped.
func TestAFailedLookupIsReportedAndNotKept(t *testing.T) {
	v := std(graph())
	src := lookingAt(graph())
	b := MustBase(v, src)
	q := mustParse(t, `edge("a", ?y) => ?y`)
	src.fail = errors.New("disk on fire")
	_, err := (SemiNaive{}).Eval(bg, q, b)
	if err == nil || err.Error() != `query: looking up edge: disk on fire` {
		t.Errorf("failed lookup: %v", err)
	}
	src.fail = nil
	if rows, err := (SemiNaive{}).Eval(bg, q, b); err != nil || col(rows, "y") != "b" {
		t.Errorf("after the failure: %v, %v; want b", col(rows, "y"), err)
	}
	ctx, cancel := context.WithCancel(bg)
	cancel()
	src.fail = ctx.Err()
	nb := *b
	nb.run = &evalRun{ctx: ctx}
	_, _, err = nb.lookup(q.Goal.Literals[0].Pos, newBinding())
	if err == nil || !strings.HasPrefix(err.Error(), "query: evaluation stopped looking up edge") || !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled lookup: %v", err)
	}
}

// Evals sharing a Base over a lookup Source run concurrently; -race checks the per-Eval cache.
func TestConcurrentEvalsLookUpOnTheirOwn(t *testing.T) {
	v := std(reversedLine(40))
	src := lookingAt(reversedLine(40))
	b := MustBase(v, src)
	want, _ := fresh(t, v, SemiNaive{}, closure+`reach("v3", ?b) => ?b`)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := (SemiNaive{}).Eval(bg, mustParse(t, closure+`reach("v3", ?b) => ?b`), b)
			if err != nil || !sameBinds(rows, want) {
				t.Errorf("concurrent Eval: %d rows, %v", len(rows), err)
			}
		}()
	}
	wg.Wait()
}

// A query that reused a relation an earlier query kept (#140) reports its cold cost: what a Base
// keeping nothing spends on it, as a fresh Base's Work shows. The run that finds it doesn't count
// toward the Base's Work. control: the reusing query's own Work is lower; and a query that reused
// nothing reports its own Work as cold.
func TestColdCostIsWhatAFreshBaseSpends(t *testing.T) {
	v := std(reversedLine(60))
	b := baseFor(v)
	first := explained(t, SemiNaive{}, allReach, b)
	if first.Cold == nil || first.Cold.Work != first.Work || first.Cold.Fetched != first.Fetched {
		t.Errorf("first query: cold %+v, work %d, fetched %d; want its own numbers", first.Cold, first.Work, first.Fetched)
	}
	bound := closure + `reach("v7", ?b) => ?b`
	before := b.Work()
	second := explained(t, SemiNaive{}, bound, b)
	_, cold := fresh(t, v, SemiNaive{}, bound)
	if rr := relationIn(second, "reach"); rr == nil || rr.How != "reused" {
		t.Fatalf("control: reach in the second query: %+v; want reused", rr)
	}
	if second.Cold == nil || second.Cold.Work != cold || second.Work >= cold {
		t.Errorf("second query: work %d, cold %+v; want cold work %d, above its own", second.Work, second.Cold, cold)
	}
	if got := b.Work() - before; got != second.Work {
		t.Errorf("the Base counted %d for the second query; want only its own %d", got, second.Work)
	}
	if !strings.Contains(second.String(), "cold: work ") {
		t.Errorf("text:\n%s", second)
	}
}

// A bound call answered from a relation the Base read whole, where a fresh Base would have looked it
// up, reports the lookup in its cold cost.
func TestColdCostCountsTheLookupsAHeldRelationSaved(t *testing.T) {
	v := std(reversedLine(40))
	b := MustBase(v, lookingAt(reversedLine(40)))
	explained(t, SemiNaive{}, `edge(?x, ?y) => ?x`, b)
	r := explained(t, SemiNaive{}, `edge("v1", ?y) => ?y`, b)
	if r.Lookups != 0 || r.Cold == nil || r.Cold.Lookups != 1 || r.Cold.Fetched != 1 {
		t.Errorf("lookups %d, cold %+v; want none, and cold one lookup fetching one tuple", r.Lookups, r.Cold)
	}
}
