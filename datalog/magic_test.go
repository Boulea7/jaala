package datalog

import (
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

// Demand is not pushed through a negation (#34): such a relation is evaluated in full, and still
// answers correctly.
func TestDemandStopsAtNegation(t *testing.T) {
	text := leftReach + `far(?a, ?b) :- reach(?a, ?b), not edge(?a, ?b); far("v0", ?x) => ?x`
	q := magic(baseFor(std(line(6))), mustParse(t, text))
	for _, r := range q.Rules {
		if isMagic(r.Head.Relation) && strings.HasPrefix(r.Head.Relation, magicPrefix+"far/") {
			t.Errorf("far reads a negation, but the rewrite made %s", displayName(r.Head.Relation))
		}
	}
	if got := col(eval(t, line(6), text), "x"); got != "v2,v3,v4,v5" {
		t.Errorf("far(v0) = %s, want v2..v5", got)
	}
	ctl := magic(baseFor(std(line(6))), mustParse(t, leftReach+`reach("v0", ?x) => ?x`))
	if reflect.DeepEqual(ctl.Rules, mustParse(t, leftReach+`reach("v0", ?x) => ?x`).Rules) {
		t.Error("control: a negation-free bound call was not rewritten either, so this test proves nothing")
	}
}

// A call that is not rewritten, here under negation, still reads the whole relation while a bound call
// to it reads the demanded part.
func TestABoundCallAndAnUnrewrittenCallCoexist(t *testing.T) {
	text := leftReach + `reach("v1", ?x), node(?y), not reach(?y, ?x) => ?x, ?y`
	if got := len(eval(t, line(6), text)); got != 4+3+2+1 {
		t.Errorf("%d rows, want 10: each of v2..v5 with the nodes not reaching it", got)
	}
	kept := false
	for _, r := range magic(baseFor(std(line(6))), mustParse(t, text)).Rules {
		kept = kept || r.Head.Relation == "reach"
	}
	if !kept {
		t.Error("the negated call needs reach's own rules, and the rewrite dropped them")
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
