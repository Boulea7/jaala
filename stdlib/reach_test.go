package stdlib_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
	"github.com/panyam/jaala/stdlib"
)

var bg = context.Background()

// chainWalk walks a line a -> b -> c -> d -> e, citing each step, and records each call.
type chainWalk struct {
	calls  []string
	ctxs   []context.Context
	visits int
}

func (w *chainWalk) walk(ctx context.Context, start ns.Value, out bool, extra []ns.Value, visit func(ns.Value, []string) error) error {
	dir := "out"
	if !out {
		dir = "back"
	}
	w.calls = append(w.calls, dir+" from "+start.S+" "+extra[0].S)
	w.ctxs = append(w.ctxs, ctx)
	const nodes = "abcde"
	cur := strings.Index(nodes, start.S)
	var path []string
	for {
		next := cur + 1
		if !out {
			next = cur - 1
		}
		if next < 0 || next >= len(nodes) {
			return nil
		}
		step := nodes[cur:cur+1] + "-" + nodes[next:next+1] // the edge cur->next, or next->cur walking back
		if !out {
			step = nodes[next:next+1] + "-" + nodes[cur:cur+1]
		}
		path = append(path, step)
		w.visits++
		if err := visit(ns.S(nodes[next:next+1]), append([]string(nil), path...)); err != nil {
			return err
		}
		cur = next
	}
}

func setup(t *testing.T) (*ns.Vocabulary, *ns.MemSource, *chainWalk) {
	t.Helper()
	src := ns.NewMemSource().Declare("node", "name")
	for _, n := range "abcde" {
		src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(string(n))}})
	}
	v := ns.MustVocabulary(src)
	if err := v.AddLanguage(datalog.Language); err != nil {
		t.Fatal(err)
	}
	w := &chainWalk{}
	if err := v.AddPredicate("graph.reach", stdlib.Reach(w.walk, ns.ArgType{Type: ns.TypeString})); err != nil {
		t.Fatal(err)
	}
	return v, src, w
}

func eval(t *testing.T, v *ns.Vocabulary, src ns.Source, text string, opts ...datalog.Option) []datalog.Row {
	t.Helper()
	rows, err := datalog.SemiNaive{}.Eval(bg, datalog.MustParse(text), datalog.MustBase(v, src), opts...)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return rows
}

func col(rows []datalog.Row, v datalog.Var) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Bind[v].S)
	}
	return strings.Join(out, ",")
}

// Out from a bound start, back from a bound end, and from the start to a bound end, stopping there:
// each answer cites its path from ?from to ?to, in order.
func TestReachRunsFromTheBoundEnd(t *testing.T) {
	for _, c := range []struct {
		query, col, want, call string
		path                   []string // the witness path of the last row
	}{
		{`graph.reach("b", ?x, "calls") => ?x`, "x", "c,d,e", "out from b calls", []string{"b-c", "c-d", "d-e"}},
		{`graph.reach(?x, "d", "calls") => ?x`, "x", "a,b,c", "back from d calls", []string{"c-d"}},
		{`graph.reach("a", "c", "calls")`, "", "", "out from a calls", []string{"a-b", "b-c"}},
	} {
		v, src, w := setup(t)
		rows := eval(t, v, src, c.query, datalog.Witnesses())
		if c.col != "" && col(rows, datalog.Var(c.col)) != c.want {
			t.Errorf("%s = %s, want %s", c.query, col(rows, datalog.Var(c.col)), c.want)
		}
		if len(w.calls) != 1 || w.calls[0] != c.call {
			t.Errorf("%s: walks %v, want one: %s", c.query, w.calls, c.call)
		}
		if got := rows[len(rows)-1].Witness[0].Cites; !reflect.DeepEqual(got, c.path) {
			t.Errorf("%s: last row's path %v, want %v from ?from to ?to", c.query, got, c.path)
		}
	}
	// back from d to a: the walk visits c, b, a, and a's path reads a -> b -> c -> d.
	v, src, _ := setup(t)
	rows := eval(t, v, src, `graph.reach("a", "d", "calls")`, datalog.Witnesses())
	if got := rows[0].Witness[0].Cites; !reflect.DeepEqual(got, []string{"a-b", "b-c", "c-d"}) {
		t.Errorf("a to d path %v", got)
	}
	v, src, _ = setup(t)
	rows = eval(t, v, src, `graph.reach(?a, "d", "calls"), ?a = "a"`, datalog.Witnesses())
	if got := rows[0].Witness[0].Cites; !reflect.DeepEqual(got, []string{"a-b", "b-c", "c-d"}) {
		t.Errorf("a walk back from d: a's path %v, want a-b, b-c, c-d (reversed to read from ?from)", got)
	}
}

func TestReachRefusesABodyThatBindsNeitherEnd(t *testing.T) {
	v, src, _ := setup(t)
	q := datalog.MustParse(`graph.reach(?a, ?b, "calls")`)
	want := `query: the query calls graph.reach(?a, ?b, "calls"), and nothing binds what graph.reach needs first: it needs ?a and "calls" bound, or ?b and "calls" bound`
	if err := datalog.Validate(q, v); err == nil || err.Error() != want {
		t.Errorf("Validate: err = %v\n want %s", err, want)
	}
	if _, err := (datalog.SemiNaive{}).Eval(bg, q, datalog.MustBase(v, src)); err == nil || err.Error() != want {
		t.Errorf("Eval: err = %v", err)
	}
}

// Inside a rule, the planner starts the walk from the bound end: Declaire's covers(?t, "x") is one
// walk back from "x", not one out from every test.
func TestReachInsideARuleWalksFromTheBoundEnd(t *testing.T) {
	v, src, w := setup(t)
	if err := v.AddModule("go", datalog.LanguageName, `test(?f) :- node(?f); covers(?t, ?f) :- test(?t), graph.reach(?t, ?f, "calls");`, ""); err != nil {
		t.Fatal(err)
	}
	rows := eval(t, v, src, `go.covers(?t, "d") => ?t`)
	if col(rows, "t") != "a,b,c" || len(w.calls) != 1 || w.calls[0] != "back from d calls" {
		t.Errorf("covers(?t, d) = %s with walks %v; want a,b,c from one walk back from d", col(rows, "t"), w.calls)
	}
}

// The query's context reaches the walk, so a host walk can stop with the request.
func TestReachPassesTheContextToTheWalk(t *testing.T) {
	v, src, w := setup(t)
	type key struct{}
	ctx := context.WithValue(bg, key{}, "req-7")
	if _, err := (datalog.SemiNaive{}).Eval(ctx, datalog.MustParse(`graph.reach("a", ?x, "calls")`), datalog.MustBase(v, src)); err != nil {
		t.Fatal(err)
	}
	if len(w.ctxs) != 1 || w.ctxs[0].Value(key{}) != "req-7" {
		t.Error("the walk did not get the query's context")
	}
	cancelled, cancel := context.WithCancel(bg)
	cancel()
	stop := stdlib.Reach(func(ctx context.Context, _ ns.Value, _ bool, _ []ns.Value, _ func(ns.Value, []string) error) error {
		return ctx.Err()
	})
	err := stop.Gen(cancelled, src, []ns.Arg{{Value: ns.S("a"), Bound: true}, {}}, func([]ns.Value, []string) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("a walk returning ctx.Err(): err = %v", err)
	}
}

// With both ends bound, the walk stops at the target rather than walking on past it.
func TestReachStopsAtABoundTarget(t *testing.T) {
	v, src, w := setup(t)
	if rows := eval(t, v, src, `graph.reach("a", "c", "calls")`); len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	if w.visits != 2 {
		t.Errorf("visited %d nodes, want 2 (b, then c, then stop)", w.visits)
	}
}

// A bound target matches a node as the engine would: 3 and 3.0 are one number.
func TestReachMatchesATargetByValue(t *testing.T) {
	three := 3.0
	b := stdlib.Reach(func(_ context.Context, _ ns.Value, _ bool, _ []ns.Value, visit func(ns.Value, []string) error) error {
		for _, n := range []float64{2, 3, 4} {
			if err := visit(ns.N(n), nil); err != nil {
				return err
			}
		}
		return nil
	})
	var got []string
	err := b.Gen(bg, nil, []ns.Arg{{Value: ns.N(1), Bound: true}, {Value: ns.Value{S: "3.0", Num: &three}, Bound: true}}, func(vals []ns.Value, _ []string) error {
		got = append(got, vals[1].S)
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, []string{"3"}) {
		t.Errorf("emitted %v, %v; want only 3", got, err)
	}
}
