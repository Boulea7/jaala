package datalog

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/panyam/jaala/ns"
)

// randomGraph is a directed graph on n nodes with each edge present at probability p, so seeds give
// cycles, self-loops, disconnected parts and dead ends without anyone choosing them. Every node also
// carries a numeric weight for the aggregate programs.
func randomGraph(seed int64, n int, p float64) *ns.MemSource {
	rnd := rand.New(rand.NewSource(seed))
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name").Declare("weight", "node", "w")
	name := func(i int) ns.Value { return ns.S(fmt.Sprintf("v%d", i)) }
	for i := 0; i < n; i++ {
		src.Add("node", ns.Tuple{Vals: []ns.Value{name(i)}, Cites: []string{fmt.Sprintf("node:%d", i)}})
		src.Add("weight", ns.Tuple{Vals: []ns.Value{name(i), ns.N(float64(rnd.Intn(10)))}})
		for j := 0; j < n; j++ {
			if rnd.Float64() < p {
				src.Add("edge", ns.Tuple{Vals: []ns.Value{name(i), name(j)}, Cites: []string{fmt.Sprintf("edge:%d-%d", i, j)}})
			}
		}
	}
	return src
}

// recursivePrograms are the shapes a fixpoint has to get right: left, right and non-linear recursion,
// mutual recursion, two recursive atoms in one body, a recursive relation read under negation and by
// an aggregate in later strata, and a chain of strata.
var recursivePrograms = []string{
	`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c); reach(?a, ?b) => ?a, ?b`,
	`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- edge(?a, ?b), reach(?b, ?c); reach("v0", ?b) => ?b`,
	`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), reach(?b, ?c); reach(?a, ?b) => ?a, ?b`,
	`even(?x) :- node(?x), ?x = "v0"; odd(?y) :- even(?x), edge(?x, ?y); even(?y) :- odd(?x), edge(?x, ?y); even(?x), odd(?x) => ?x`,
	// A fact only the second recursive atom's delta can find: even(x) is old by the round odd(x) is new.
	`even(?x) :- node(?x), ?x = "v0"; odd(?y) :- even(?x), edge(?x, ?y); even(?y) :- odd(?x), edge(?x, ?y); ` +
		`both(?x) :- even(?x), odd(?x); both(?x) => ?x`,
	`sg(?x, ?y) :- edge(?p, ?x), edge(?p, ?y); sg(?x, ?y) :- edge(?p, ?x), sg(?p, ?q), edge(?q, ?y); sg(?x, ?y) => ?x, ?y`,
	`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c); cyclic(?a) :- reach(?a, ?a); ` +
		`stuck(?a) :- node(?a), not cyclic(?a); stuck(?a) => ?a`,
	`reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c); reach(?a, ?b), weight(?b, ?w) => ?a, count(distinct ?b), sum(?w)`,
	`r1(?a, ?b) :- edge(?a, ?b); r2(?a, ?c) :- r1(?a, ?b), r1(?b, ?c); r2(?a, ?c) :- r2(?a, ?b), r1(?b, ?c); ` +
		`far(?a, ?b) :- r2(?a, ?b), not r1(?a, ?b); far(?a, ?b) => ?a, ?b`,
	`t(?a, ?b) :- edge(?a, ?b); t(?a, ?c) :- t(?a, ?b), t(?b, ?c), ?a != ?c; t(?a, ?b), not edge(?a, ?b) => ?a, ?b`,
}

// SemiNaive must answer exactly as Naive does, rows and citations, on every recursive shape over many
// random graphs. both panics on the first disagreement and prints the program.
func TestSemiNaiveAgreesWithNaiveOnRandomGraphs(t *testing.T) {
	compared := 0
	for seed := int64(1); seed <= 40; seed++ {
		src := randomGraph(seed, 4+int(seed%9), 0.08+float64(seed%5)*0.06)
		b := baseFor(std(src))
		for _, text := range recursivePrograms {
			if _, err := both(mustParse(t, text), b); err != nil {
				t.Fatalf("seed %d: %s: %v", seed, text, err)
			}
			compared++
		}
	}
	if compared != 40*len(recursivePrograms) {
		t.Errorf("compared %d programs, want %d", compared, 40*len(recursivePrograms))
	}
}

// chainOf is a path v0 -> v1 -> ... -> v(n-1).
func chainOf(n int) *ns.MemSource {
	src := ns.NewMemSource().Declare("edge", "from", "to")
	for i := 0; i+1 < n; i++ {
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", i)), ns.S(fmt.Sprintf("v%d", i+1))}})
	}
	return src
}

// The point of semi-naive evaluation, as a ratio rather than a timing: doubling a chain under a
// transitive closure multiplies Naive's work by about 8 (n rounds of n² derivations) and SemiNaive's
// by about 4 (each of the n² facts joined once).
func TestSemiNaiveClosureGrowsQuadratically(t *testing.T) {
	q := mustParse(t, `reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c); reach(?a, ?b) => ?a, ?b`)
	work := func(ev Evaluator, n int) int64 {
		b := baseFor(std(chainOf(n)))
		rows, err := ev.Eval(q, b)
		if err != nil || len(rows) != n*(n-1)/2 {
			t.Fatalf("n=%d: %d rows, %v", n, len(rows), err)
		}
		return b.Work()
	}
	naive := float64(work(Naive{}, 80)) / float64(work(Naive{}, 40))
	semi := float64(work(SemiNaive{}, 80)) / float64(work(SemiNaive{}, 40))
	if semi > 4.5 {
		t.Errorf("SemiNaive work grew %.1fx when the chain doubled, want about 4x", semi)
	}
	if naive < 7 {
		t.Errorf("control: Naive work grew only %.1fx, so this chain is not exercising the fixpoint", naive)
	}
}

// In the round q(x) is new, p is new somewhere else too, and p(x) has been known since round zero. Only
// the variant reading q's delta, the second recursive atom, can derive both(x), and it must run even
// though the first atom's delta is not empty.
func TestEveryRecursiveAtomGetsItsDeltaVariant(t *testing.T) {
	src := ns.NewMemSource().Declare("base1", "n").Declare("base2", "n").Declare("edge1", "a", "b").Declare("edge2", "a", "b")
	src.Add("base1", ns.Tuple{Vals: []ns.Value{ns.S("x")}}).Add("base1", ns.Tuple{Vals: []ns.Value{ns.S("a0")}}).Add("base2", ns.Tuple{Vals: []ns.Value{ns.S("b0")}})
	for _, e := range [][2]string{{"a0", "a1"}, {"a1", "a2"}, {"a2", "a3"}, {"a3", "a4"}, {"a4", "a5"}} {
		src.Add("edge1", ns.Tuple{Vals: []ns.Value{ns.S(e[0]), ns.S(e[1])}})
	}
	for _, e := range [][2]string{{"b0", "b1"}, {"b1", "b2"}, {"b2", "x"}} {
		src.Add("edge2", ns.Tuple{Vals: []ns.Value{ns.S(e[0]), ns.S(e[1])}})
	}
	rows := eval(t, src, `p(?x) :- base1(?x); p(?y) :- p(?x), edge1(?x, ?y); q(?x) :- base2(?x); q(?y) :- q(?x), edge2(?x, ?y); `+
		`both(?x) :- p(?x), q(?x); both(?x) => ?x`)
	if got := col(rows, "x"); got != "x" {
		t.Errorf("both = %q, want x", got)
	}
}
