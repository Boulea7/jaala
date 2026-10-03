package datalog

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/panyam/jaala/ns"
)

// coversModule is Declaire's go.test and go.covers in miniature, over line()'s nodes and a two-mode
// walk.
const coversModule = `
test(?f) :- node(?f);
covers(?t, ?f) :- test(?t), walk(?t, ?f);
`

func coversVocabulary(t *testing.T, n int) (*ns.Vocabulary, *ns.MemSource, *[][2]bool) {
	t.Helper()
	src := line(n)
	v := std(src)
	calls := walker(t, v, [][]bool{{true, false}, {false, true}})
	if err := v.AddModule("go", LanguageName, coversModule, ""); err != nil {
		t.Fatal(err)
	}
	return v, src, calls
}

// A constant at the call reaches the walk inside go.covers, so it runs once, back from the bound end,
// instead of once per test.
func TestABoundArgumentReachesTheGeneratorInsideARelation(t *testing.T) {
	for _, c := range []struct {
		name, query string
	}{
		{"a constant", `go.covers(?t, "v5") => ?t`},
		{"a variable bound by the caller", `node(?f), ?f = "v5", go.covers(?t, ?f) => ?t`},
	} {
		v, src, calls := coversVocabulary(t, 40)
		want, err := Naive{}.Eval(bg, mustParse(t, c.query), MustBase(v, src))
		if err != nil || col(want, "t") != "v0,v1,v2,v3,v4" {
			t.Fatalf("%s: Naive = %v, %v", c.name, want, err)
		}
		if len(*calls) < 40 {
			t.Errorf("%s: control: Naive walked %d times, want at least once per test (40)", c.name, len(*calls))
		}
		*calls = nil
		got, err := SemiNaive{}.Eval(bg, mustParse(t, c.query), MustBase(v, src))
		if err != nil || !reflect.DeepEqual(binds(got), binds(want)) {
			t.Errorf("%s: SemiNaive = %v, %v; want %v", c.name, got, err, want)
		}
		if len(*calls) != 1 || (*calls)[0] != [2]bool{false, true} {
			t.Errorf("%s: SemiNaive walk calls = %v, want one, back from the bound end", c.name, *calls)
		}
	}
}

// A derived relation is a set. Inlined into a goal that counts bindings, its body would count each
// net once per edge into it; the documented has_tp idiom (see Aggregate) depends on it not.
func TestInliningLeavesABindingCountAlone(t *testing.T) {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name")
	for _, n := range []string{"a", "b", "c", "d"} {
		src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(n)}})
	}
	for _, e := range [][2]string{{"a", "c"}, {"b", "c"}, {"a", "d"}} {
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(e[0]), ns.S(e[1])}})
	}
	rows := eval(t, src, `has(?n) :- edge(?x, ?n); node(?n), has(?n) => count(?n)`)
	if got := rows[0].Bind["count(n)"].S; got != "2" {
		t.Errorf("count = %s, want 2 (c and d, each once)", got)
	}
	q := unfold(baseFor(std(src)), mustParse(t, `has(?n) :- edge(?x, ?n); node(?n), has(?n) => count(distinct ?n)`))
	if strings.Contains(q.Goal.String(), "has(") {
		t.Errorf("a distinct count cannot see duplicates, so has should be inlined: goal %s", q.Goal)
	}
}

// Each call unfold must leave alone, with a positive control that inlines.
func TestWhatUnfoldLeavesAlone(t *testing.T) {
	b := baseFor(std(graph()))
	for _, c := range []struct {
		why, query string
		inlined    bool
	}{
		{"control: one plain rule", `p(?x) :- node(?x); p(?y), edge(?y, ?z) => ?z`, true},
		{"control: a head constant met by a constant", `p(?x, "k") :- node(?x); p(?y, "k") => ?y`, true},
		{"negated", `p(?x) :- edge(?x, ?y); node(?n), not p(?n) => ?n`, false},
		{"recursive", `p(?a, ?b) :- edge(?a, ?b); p(?a, ?c) :- p(?a, ?b), edge(?b, ?c); p("a", ?z) => ?z`, false},
		// One rule each, so only the recursion guard keeps inlining from going round forever.
		{"recursive through one rule each", `p(?x) :- node(?x), q(?x); q(?x) :- edge(?x, ?y), p(?y); p(?n) => ?n`, false},
		{"several rules", `p(?x) :- edge(?x, ?y); p(?x) :- edge(?y, ?x); p(?n) => ?n`, false},
		{"a head constant met by a variable", `p(?x, "k") :- node(?x); p(?y, ?w) => ?y, ?w`, false},
		{"a head constant met by another constant", `p(?x, "k") :- node(?x); p(?y, "j") => ?y`, false},
		{"a repeated head variable", `p(?x, ?x) :- node(?x); p(?y, ?z) => ?y`, false},
	} {
		done := make(chan Query, 1)
		go func() { done <- unfold(b, mustParse(t, c.query)) }()
		var q Query
		select {
		case q = <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: unfold did not terminate", c.why)
		}
		got := !strings.Contains(q.Goal.String(), "p(")
		if got != c.inlined {
			t.Errorf("%s: inlined = %v, want %v (goal %s)", c.why, got, c.inlined, q.Goal)
		}
	}
}

// A recursive rule's body runs every round of the fixpoint, so a join inlined into it is redone each
// time, where the relation would be derived once and indexed (#97). control: the goal's call to the
// same relation is still inlined.
func TestUnfoldLeavesARecursiveBodyAlone(t *testing.T) {
	q := unfold(baseFor(std(graph())), mustParse(t, `s(?a, ?b) :- edge(?a, ?b), node(?b); `+
		`r(?a, ?b) :- s(?a, ?b); r(?a, ?c) :- r(?a, ?b), s(?b, ?c); r(?x, ?y), s(?y, ?z) => ?x, ?z`))
	for _, r := range q.Rules {
		if r.Head.Relation == "r" && !strings.Contains(r.Body.String(), "s(") {
			t.Errorf("s was inlined into r's rule: %s", r)
		}
	}
	if strings.Contains(q.Goal.String(), "s(") {
		t.Errorf("control: the goal's call to s should be inlined: %s", q.Goal)
	}
}

// Once every call to a relation is inlined it is not materialized, and nor is what only it read; a
// rule the goal never reached stays, as a query's own rules always do.
func TestUnfoldDropsWhatTheGoalNoLongerReaches(t *testing.T) {
	q := unfold(baseFor(std(graph())), mustParse(t,
		`two(?a, ?c) :- one(?a, ?b), one(?b, ?c); one(?a, ?b) :- edge(?a, ?b); `+
			`multi(?x) :- node(?x); multi(?x) :- edge(?x, _); via(?x) :- multi(?x); `+
			`unused(?x) :- node(?x); two("a", ?c), via(?c) => ?c`))
	var heads []string
	for _, r := range q.Rules {
		heads = append(heads, r.Head.Relation)
	}
	sort.Strings(heads)
	if got := strings.Join(heads, ","); got != "multi,multi,unused" {
		t.Errorf("rules kept = %s, want multi (still called, from the inlined via) and unused (never reached)", got)
	}
}

// The answer's columns are the written goal's, not the inlined one's: the variable an inlined body
// adds (has's ?x) is not one of them.
func TestUnfoldKeepsTheWrittenColumns(t *testing.T) {
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, `has(?n) :- edge(?x, ?n); has(?n)`), baseFor(std(graph())))
	if err != nil || col(rows, "n") != "b,c,d" {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	for _, r := range rows {
		if len(r.Bind) != 1 {
			t.Errorf("row binds %v, want only ?n", r.Bind)
		}
	}
}
