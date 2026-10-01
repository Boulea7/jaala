package datalog

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

const (
	leftReach  = `reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c); `
	rightReach = `reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- edge(?a, ?b), reach(?b, ?c); `
)

func workOf(t *testing.T, ev Evaluator, src ns.Source, q string) int64 {
	t.Helper()
	b := baseFor(std(src))
	if _, err := ev.Eval(bg, mustParse(t, q), b); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return b.Work()
}

// Asked about one start, a left-linear closure derives only what that start reaches: its work grows
// with the chain, not with the chain's square, and asked about one end it walks back only as far as
// that end.
func TestDemandMakesABoundClosureLinear(t *testing.T) {
	q := leftReach + `reach("v0", ?x) => ?x`
	ratio := float64(workOf(t, SemiNaive{}, line(400), q)) / float64(workOf(t, SemiNaive{}, line(200), q))
	if ratio > 2.5 {
		t.Errorf("bound-start closure work grew %.1fx when the chain doubled, want about 2x", ratio)
	}
	full := float64(workOf(t, SemiNaive{WrittenOrder: true}, line(400), q)) / float64(workOf(t, SemiNaive{WrittenOrder: true}, line(200), q))
	if full < 3.5 {
		t.Errorf("control: without demand the work grew only %.1fx, so the closure is not being built in full", full)
	}
	q = leftReach + `reach(?x, "v5") => ?x`
	if near, far := workOf(t, SemiNaive{}, line(200), q), workOf(t, SemiNaive{}, line(400), q); near != far {
		t.Errorf("bound-end closure work = %d on 200 nodes and %d on 400, want the same (it walks back 5)", near, far)
	}
}

// Asked about one chain of twenty, right-linear recursion does the work of that chain alone.
func TestDemandHelpsRightLinearRecursionOnAPartOfTheGraph(t *testing.T) {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name")
	for c := 0; c < 20; c++ {
		for i := 0; i < 30; i++ {
			src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("c%d_%d", c, i))}})
			if i+1 < 30 {
				src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("c%d_%d", c, i)), ns.S(fmt.Sprintf("c%d_%d", c, i+1))}})
			}
		}
	}
	q := rightReach + `reach("c0_0", ?x) => ?x`
	if with, without := workOf(t, SemiNaive{}, src, q), workOf(t, SemiNaive{WrittenOrder: true}, src, q); with*20 > without {
		t.Errorf("work with demand %d, without %d; want under a twentieth, since only one chain of twenty is asked about", with, without)
	}
}

// A magic tuple says what was asked, not what produced an answer. Here s1 and s2 both reach z, and z
// is demanded first on s1's behalf; s2's answer must still cite only s2's own facts.
func TestDemandLeavesNoCitationsOnTheAnswer(t *testing.T) {
	src := ns.NewMemSource().Declare("src", "s").Declare("q", "s", "z").Declare("base", "z", "y").Declare("alt", "z", "y")
	for _, s := range []string{"s1", "s2"} {
		src.Add("src", ns.Tuple{Vals: []ns.Value{ns.S(s)}, Cites: []string{"src:" + s}})
		src.Add("q", ns.Tuple{Vals: []ns.Value{ns.S(s), ns.S("z")}, Cites: []string{"q:" + s}})
	}
	src.Add("base", ns.Tuple{Vals: []ns.Value{ns.S("z"), ns.S("y")}, Cites: []string{"base:zy"}})
	text := `r(?z, ?y) :- base(?z, ?y); r(?z, ?y) :- alt(?z, ?y); src(?x), q(?x, ?z), r(?z, ?y) => ?x, ?y`
	if !strings.Contains(fmt.Sprint(magic(baseFor(std(src)), unfold(baseFor(std(src)), mustParse(t, text))).Rules), magicPrefix) {
		t.Fatal("control: r is not called for demand, so this test proves nothing")
	}
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, text), baseFor(std(src)))
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	for _, r := range rows {
		x := r.Bind["x"].S
		cites := append([]string(nil), r.Cites...)
		sort.Strings(cites)
		if want := []string{"base:zy", "q:" + x, "src:" + x}; !reflect.DeepEqual(cites, want) {
			t.Errorf("row %s cites %v, want %v", x, cites, want)
		}
	}
}

// Demand passes through a relation that uses negation (#34): asked about one start, far derives
// only what that start reaches, so its work grows with the chain. far has two rules so that it is
// not inlined, which would hand the demand to reach without passing through far.
func TestDemandPassesThroughANegatingRelation(t *testing.T) {
	text := leftReach + `far(?a, ?b) :- reach(?a, ?b), not edge(?a, ?b); far(?a, ?b) :- reach(?a, ?b), node(?b), not edge(?a, ?b); far("v0", ?x) => ?x`
	if got := col(eval(t, line(6), text), "x"); got != "v2,v3,v4,v5" {
		t.Errorf("far(v0) = %s, want v2..v5", got)
	}
	if ratio := float64(workOf(t, SemiNaive{}, line(400), text)) / float64(workOf(t, SemiNaive{}, line(200), text)); ratio > 2.5 {
		t.Errorf("far(v0) work grew %.1fx when the chain doubled, want about 2x", ratio)
	}
	if full := float64(workOf(t, SemiNaive{WrittenOrder: true}, line(400), text)) / float64(workOf(t, SemiNaive{WrittenOrder: true}, line(200), text)); full < 3.5 {
		t.Errorf("control: without demand far's work grew only %.1fx, so reach is not being built in full", full)
	}
}

// Demand goes into a negated call: agni's nocov("C12") :- p(?c), not cov(?c) derives cov only at C12.
// Here cov is "reaches anything", over a left-linear closure, so in full it is every pair.
func TestDemandGoesIntoANegatedCall(t *testing.T) {
	const rules = leftReach + `cov(?a) :- reach(?a, ?b); nocov(?c) :- node(?c), not cov(?c); `
	if got := col(eval(t, line(6), rules+`nocov(?c) => ?c`), "c"); got != "v5" {
		t.Errorf("nocov = %s, want v5 (the end of the line reaches nothing)", got)
	}
	if got := len(eval(t, line(6), rules+`nocov("v5")`)) + len(eval(t, line(6), rules+`nocov("v2")`)); got != 1 {
		t.Errorf("nocov(v5) and nocov(v2): %d rows, want 1", got)
	}
	q := magic(baseFor(std(line(6))), mustParse(t, rules+`nocov("v0")`))
	into := false
	for _, r := range q.Rules {
		into = into || r.Head.Relation == magicName("cov", "b")
	}
	if !into {
		t.Fatal("no demand rule for cov: the negated call was not rewritten")
	}
	goal := rules + `nocov("v0")`
	if ratio := float64(workOf(t, SemiNaive{}, line(400), goal)) / float64(workOf(t, SemiNaive{}, line(200), goal)); ratio > 2.5 {
		t.Errorf("nocov(v0) work grew %.1fx when the chain doubled, want about 2x", ratio)
	}
	if full := float64(workOf(t, SemiNaive{WrittenOrder: true}, line(400), goal)) / float64(workOf(t, SemiNaive{WrittenOrder: true}, line(200), goal)); full < 3.5 {
		t.Errorf("control: without demand the work grew only %.1fx, so cov is not being built in full", full)
	}
}

// Demand into a negation would make this program unstratifiable: p is recursive, so the demand for q
// at ?z depends on p, and p negates q. The rewrite is made again with q read in full, and answers as
// Naive does.
func TestDemandIntoANegationFallsBackWhenItWouldNotStratify(t *testing.T) {
	text := `q(?y) :- weight(?y, ?w), ?w > 5; p(?x, ?y) :- edge(?x, ?y), not q(?y); p(?x, ?z) :- p(?x, ?y), edge(?y, ?z), not q(?z); p("v0", ?z) => ?z`
	b := baseFor(std(randomGraph(3, 10, 0.3)))
	if _, err := stratify(magicWith(b, mustParse(t, text), true).Rules, derivedArity(magicWith(b, mustParse(t, text), true).Rules)); err == nil {
		t.Fatal("control: demand into the negation stratifies here, so the fallback is not exercised")
	}
	q := magic(b, mustParse(t, text))
	if _, err := stratify(q.Rules, derivedArity(q.Rules)); err != nil {
		t.Fatalf("the fallback does not stratify: %v", err)
	}
	for _, r := range q.Rules {
		if r.Head.Relation == magicName("q", "b") {
			t.Errorf("the fallback still demands q: %s", r)
		}
	}
	for seed := int64(1); seed <= 10; seed++ {
		if _, err := both(mustParse(t, text), baseFor(std(randomGraph(seed, 10, 0.3)))); err != nil {
			t.Errorf("seed %d: %v", seed, err)
		}
	}
}

// Bound calls over random graphs, through every recursive shape, answer as Naive does: through both()
// as written, and planned from a shuffled body against Naive on the written one.
func TestDemandAgreesWithNaiveOnRandomGraphs(t *testing.T) {
	programs := []string{
		leftReach + `reach("v0", ?x) => ?x`,
		rightReach + `reach("v1", ?x) => ?x`,
		rightReach + `reach("v0", ?x), reach("v2", ?x) => ?x`,
		`p(?a, ?k, ?b) :- edge(?a, ?b), weight(?a, ?k); p(?a, ?k, ?c) :- edge(?a, ?b), p(?b, ?k, ?c); p("v1", ?k, ?x) => ?k, ?x`,
		`r(?a, ?b) :- edge(?a, ?b); r(?a, ?a) :- node(?a); r(?a, ?c) :- edge(?a, ?b), weight(?b, ?w), ?w > 3, r(?b, ?c); r("v0", ?x) => ?x`,
		`r(?a) :- node(?a), ?a = "v3"; r(?a) :- edge(?a, ?b), r(?b); r("v0")`,
		leftReach + `reach(?x, "v2") => ?x`,
		`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), reach(?b, ?c); reach("v0", ?x) => ?x`,
		`sg(?x, ?y) :- edge(?p, ?x), edge(?p, ?y); sg(?x, ?y) :- edge(?p, ?x), sg(?p, ?q), edge(?q, ?y); sg("v1", ?y) => ?y`,
		`even(?x) :- node(?x), ?x = "v0"; odd(?y) :- even(?x), edge(?x, ?y); even(?y) :- odd(?x), edge(?x, ?y); odd("v3")`,
		leftReach + `node(?s), weight(?s, ?w), ?w > 6, reach(?s, ?x) => ?s, ?x`,
		leftReach + `two(?a, ?c) :- reach(?a, ?b), reach(?b, ?c); two("v0", ?c), weight(?c, ?w) => ?c, max(?w)`,
		leftReach + `far(?a, ?b) :- reach(?a, ?b), not edge(?a, ?b); far("v0", ?x) => ?x`,
		leftReach + `cov(?a) :- reach(?a, ?b), weight(?b, ?w), ?w > 6; nocov(?c) :- node(?c), not cov(?c); nocov("v1")`,
		leftReach + `cov(?a) :- reach(?a, ?b), weight(?b, ?w), ?w > 6; node(?c), not cov(?c) => ?c`,
		`q(?y) :- weight(?y, ?w), ?w > 5; p(?x, ?y) :- edge(?x, ?y), not q(?y); p(?x, ?z) :- p(?x, ?y), edge(?y, ?z), not q(?z); p("v0", ?z) => ?z`,
		leftReach + `node(?s), weight(?s, ?w), reach(?s, ?x) => ?s, count(?x)`,
	}
	for seed := int64(1); seed <= 25; seed++ {
		b := baseFor(std(randomGraph(seed, 5+int(seed%8), 0.1+float64(seed%4)*0.07)))
		rnd := rand.New(rand.NewSource(seed))
		for _, text := range programs {
			want, err := both(mustParse(t, text), b)
			if err != nil {
				t.Fatalf("seed %d: %s: %v", seed, text, err)
			}
			got, err := SemiNaive{}.Eval(bg, shuffle(mustParse(t, text), rnd), b)
			if err != nil || !reflect.DeepEqual(rowSet(got), rowSet(want)) {
				t.Errorf("seed %d, shuffled %s:\n got  %v %v\n want %v", seed, text, rowSet(got), err, rowSet(want))
			}
		}
	}
}

// An error in a rule names the rule as written, not the relation a rewrite renamed it to: adorned for
// demand (left-linear here) or factored (right-linear).
func TestARewrittenRuleErrsUnderItsOwnName(t *testing.T) {
	want := `query: rule "r" head variable ?b is not bound by a positive body relation`
	for _, rec := range []string{`r(?a, ?b), edge(?b, ?c)`, `edge(?a, ?b), r(?b, ?c)`} {
		text := `r(?a, ?b) :- edge(?a, ?b); r(?a, ?b) :- node(?a), ?a = ?b; r(?a, ?c) :- ` + rec + `; r("v0", ?x) => ?x`
		for _, ev := range evaluators {
			if _, err := ev.Eval(bg, mustParse(t, text), baseFor(std(line(5)))); err == nil || err.Error() != want {
				t.Errorf("%T, recursing %s: err = %v, want %s", ev, rec, err, want)
			}
		}
	}
}

// A prefix that passes demand on runs once (#54): the walk before r is stored in a supplementary
// relation that r's demand and the rest of the goal both read. A goal whose aggregate counts bindings
// keeps its own prefix (a supplementary relation is a set), so there the walk runs twice.
func TestAPrefixThatPassesDemandRunsOnce(t *testing.T) {
	const rules = `r(?a, ?b) :- edge(?a, ?b); r(?a, ?b) :- edge(?a, ?c), edge(?c, ?b); `
	for _, c := range []struct {
		goal  string
		walks int
	}{
		{`walk("v1", ?y), r(?y, ?z) => ?y, ?z`, 1},
		{`walk("v1", ?y), r(?y, ?z) => count(?z)`, 2},
	} {
		v := std(line(6))
		calls := walker(t, v, [][]bool{{true, false}, {false, true}})
		if _, err := (SemiNaive{}).Eval(bg, mustParse(t, rules+c.goal), baseFor(v)); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != c.walks {
			t.Errorf("%s: %d walks, want %d", c.goal, len(*calls), c.walks)
		}
	}
	v := std(line(6))
	walker(t, v, [][]bool{{true, false}, {false, true}})
	if got, want := rowSet(evalReg(t, v, rules+`walk("v1", ?y), r(?y, ?z) => ?y, ?z`)), 3+2; len(got) != want {
		t.Errorf("%d rows, want %d: %v", len(got), want, got)
	}
}

// A supplementary tuple is a prefix's result, not demand, so it keeps the prefix's citations: each
// answer of a right-linear closure from a start bound through a variable cites its whole path.
func TestASupplementaryTupleKeepsItsCitations(t *testing.T) {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name")
	for i := 0; i < 5; i++ {
		src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", i))}})
		if i < 4 {
			src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", i)), ns.S(fmt.Sprintf("v%d", i+1))}, Cites: []string{fmt.Sprintf("e%d", i)}})
		}
	}
	text := rightReach + `node(?s), ?s = "v0", reach(?s, ?x) => ?x`
	if !strings.Contains(fmt.Sprint(magic(baseFor(std(src)), mustParse(t, text)).Rules), supPrefix) {
		t.Fatal("control: no supplementary relation, so this test proves nothing")
	}
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, text), baseFor(std(src)))
	if err != nil || len(rows) != 4 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	for _, r := range rows {
		var n int
		fmt.Sscanf(r.Bind["x"].S, "v%d", &n)
		var want []string
		for i := 0; i < n; i++ {
			want = append(want, fmt.Sprintf("e%d", i))
		}
		cites := append([]string(nil), r.Cites...)
		sort.Strings(cites)
		if !reflect.DeepEqual(cites, want) {
			t.Errorf("reach(v0, %s) cites %v, want %v", r.Bind["x"].S, cites, want)
		}
	}
}

// A witnessed Eval keeps the demand a bound call carries (#53): Declaire's go.covers, whose test uses
// a negation, walks once, answers as an unwitnessed Eval does, and its witness still shows covers' own
// node and rule, with the walk's citations in walk order.
func TestAWitnessedBoundCallKeepsItsDemand(t *testing.T) {
	const text = `test(?f) :- attr(?f, "role", "test"), not attr(?f, "receiver", _); ` +
		`covers(?t, ?f) :- test(?t), hop(?t, ?f); covers(?t, "v5") => ?t`
	run := func(opts ...Option) ([]Row, int) {
		v := std(tested())
		walks := 0
		if err := v.AddPredicate("hop", ns.Builtin{Arity: 2, Modes: [][]bool{{true, false}, {false, true}}, Gen: func(_ context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			walks++
			var end int
			if !args[1].Bound {
				return nil // this test only binds the far end
			}
			fmt.Sscanf(args[1].Value.S, "v%d", &end)
			for start := end - 1; start >= 0; start-- {
				var path []string
				for i := start; i < end; i++ {
					path = append(path, fmt.Sprintf("e%d-%d", i, i+1))
				}
				if err := emit([]ns.Value{ns.S(fmt.Sprintf("v%d", start)), args[1].Value}, path); err != nil {
					return err
				}
			}
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
		rows, err := SemiNaive{}.Eval(bg, mustParse(t, text), baseFor(v), opts...)
		if err != nil {
			t.Fatal(err)
		}
		return rows, walks
	}
	plain, _ := run()
	rows, walks := run(Witnesses())
	if walks != 1 {
		t.Errorf("witnessed: %d walks, want 1", walks)
	}
	if !reflect.DeepEqual(rowSet(rows), rowSet(plain)) || col(rows, "t") != "v0,v1,v2" {
		t.Errorf("witnessed rows %v, unwitnessed %v; want the same, v0..v2", rowSet(rows), rowSet(plain))
	}
	for _, r := range rows {
		if len(r.Witness) != 1 || r.Witness[0].Relation != "covers" || r.Witness[0].Rule != `covers(?t, ?f) :- test(?t), hop(?t, ?f)` {
			t.Fatalf("row %s: witness %+v, want covers' own node with its rule as written", r.Bind["t"].S, r.Witness)
		}
		kids := r.Witness[0].Children
		if len(kids) != 2 || kids[0].Relation != "test" || kids[1].Relation != "hop" {
			t.Fatalf("row %s: covers' children %+v, want test then hop, as written", r.Bind["t"].S, kids)
		}
		var start int
		fmt.Sscanf(r.Bind["t"].S, "v%d", &start)
		var want []string
		for i := start; i < 5; i++ {
			want = append(want, fmt.Sprintf("e%d-%d", i, i+1))
		}
		if !reflect.DeepEqual(kids[1].Cites, want) {
			t.Errorf("row %s: hop cites %v, want %v in walk order", r.Bind["t"].S, kids[1].Cites, want)
		}
	}
}
