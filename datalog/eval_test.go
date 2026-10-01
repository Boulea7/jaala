package datalog

import (
	"context"
	"fmt"
	"github.com/panyam/jaala/ns"
	"strings"
	"testing"
)

func TestJoinProjectsAndCarriesCitations(t *testing.T) {
	rows := eval(t, graph(), `edge(?a,?b), edge(?b,?c) => ?a, ?c`)
	if got := col(rows, "a") + "|" + col(rows, "c"); got != "a,b|c,d" {
		t.Fatalf("two-hop pairs = %s, want a,b|c,d", got)
	}
	for _, r := range rows {
		if len(r.Cites) != 2 {
			t.Errorf("row %v cites %v, want the two edges that produced it", r.Bind, r.Cites)
		}
	}
}

func TestRecursiveRuleReachesFixpoint(t *testing.T) {
	rows := eval(t, graph(), `reach(?a,?b) :- edge(?a,?b); reach(?a,?c) :- reach(?a,?b), edge(?b,?c); reach("a", ?x) => ?x`)
	if got := col(rows, "x"); got != "b,c,d" {
		t.Errorf("reach from a = %s, want b,c,d", got)
	}
}

func TestNegationIsAFilterOverTheAnchoredVariable(t *testing.T) {
	rows := eval(t, graph(), `node(?n), not edge(?n, ?_) => ?n`)
	if got := col(rows, "n"); got != "d,x" {
		t.Errorf("nodes with no out-edge = %s, want d,x", got)
	}
}

func TestUnanchoredNegationIsAnError(t *testing.T) {
	err := evalErr(graph(), `node(?n), not edge(?p, ?q) => ?n`)
	if err == nil || !strings.Contains(err.Error(), "shares no variable") {
		t.Errorf("err = %v, want the unanchored-negation error", err)
	}
}

func TestRecursionThroughNegationIsRejected(t *testing.T) {
	err := evalErr(graph(), `p(?x) :- node(?x), not q(?x); q(?x) :- node(?x), not p(?x); p(?x) => ?x`)
	if err == nil || !strings.Contains(err.Error(), "not stratifiable") {
		t.Errorf("err = %v, want a stratification error", err)
	}
}

func TestStratifiedNegationReadsAFullyDerivedRelation(t *testing.T) {
	rows := eval(t, graph(), `has_out(?n) :- edge(?n, ?_); sink(?n) :- node(?n), not has_out(?n); sink(?n) => ?n`)
	if got := col(rows, "n"); got != "d,x" {
		t.Errorf("sinks = %s, want d,x", got)
	}
}

func TestNumericComparison(t *testing.T) {
	rows := eval(t, graph(), `weight(?n, ?w), ?w > 3 => ?n`)
	if got := col(rows, "n"); got != "d,x" {
		t.Errorf("heavy nodes = %s, want d,x", got)
	}
}

func TestAggregateGroupsAndHaving(t *testing.T) {
	rows := eval(t, graph(), `edge(?a, ?b) => ?a, count(?b)`)
	if len(rows) != 3 {
		t.Fatalf("groups = %v, want one per node with an out-edge", rows)
	}
	rows = eval(t, graph(), `weight(?n, ?w) => sum(?w), max(?w), min(?w)`)
	if b := rows[0].Bind; b["sum(w)"].S != "15" || b["max(w)"].S != "5" || b["min(w)"].S != "1" {
		t.Errorf("sum/max/min = %v, want 15/5/1", b)
	}
	rows = eval(t, graph(), `edge(?a, ?b) => ?a having count(?b) > 1`)
	if len(rows) != 0 {
		t.Errorf("having count > 1 = %v, want none (every node has at most one out-edge)", rows)
	}
}

// An aggregate-only projection is one group by definition and answers even when nothing matched, as
// SQL's COUNT(*) does (agni issue 726). A grouped one has no key to name a group by, so it does not.
func TestAggregateOverNothing(t *testing.T) {
	rows := eval(t, graph(), `edge(?a, "zzz") => count(?a), sum(?a), list(?a)`)
	if len(rows) != 1 || rows[0].Bind["count(a)"].S != "0" || rows[0].Bind["sum(a)"].Num != nil || rows[0].Bind["list(a)"].S != "" {
		t.Errorf("rows = %+v, want one row: count 0, sum with no value, list empty", rows)
	}
	if rows := eval(t, graph(), `edge(?a, "zzz") => ?a, count(?a)`); len(rows) != 0 {
		t.Errorf("grouped over nothing = %v, want no rows", rows)
	}
}

func TestUnknownRelationSuggestsAKnownOne(t *testing.T) {
	err := evalErr(graph(), `egde(?a, ?b)`)
	if err == nil || !strings.Contains(err.Error(), `did you mean "edge"`) {
		t.Errorf("err = %v, want a suggestion of edge", err)
	}
}

type hintedEmpty struct{ *ns.MemSource }

func (hintedEmpty) NoVocabularyHint() string { return "install the widgets" }

// A Source serving nothing cannot call a relation unknown, so the error says the vocabulary is
// missing instead, in the host's words when it offers them.
func TestEmptyVocabularySaysSoRatherThanGuessing(t *testing.T) {
	err := evalErr(ns.NewMemSource(), `egde(?a, ?b)`)
	if err == nil || !strings.Contains(err.Error(), "no relations are installed") {
		t.Errorf("err = %v, want the no-vocabulary hint", err)
	}
	err = evalErr(hintedEmpty{ns.NewMemSource()}, `egde(?a, ?b)`)
	if err == nil || !strings.Contains(err.Error(), "install the widgets") {
		t.Errorf("err = %v, want the host's hint", err)
	}
}

func TestWrongArityIsAnError(t *testing.T) {
	err := evalErr(graph(), `edge(?a)`)
	if err == nil || !strings.Contains(err.Error(), "takes 2 args") {
		t.Errorf("err = %v, want an arity error", err)
	}
}

func TestClosedDomainRejectsAConstantOutsideIt(t *testing.T) {
	src := ns.NewMemSource().DeclareSchema("role", ns.Schema{Arity: 2, Labels: []string{"node", "role"}, Types: []ns.ArgType{{}, {Domain: []string{"source", "sink"}}}})
	src.Add("role", ns.Tuple{Vals: []ns.Value{ns.S("a"), ns.S("source")}})
	err := evalErr(src, `role(?n, "sorce")`)
	if err == nil || !strings.Contains(err.Error(), `did you mean "source"`) {
		t.Errorf("err = %v, want a domain error suggesting source", err)
	}
	if rows := eval(t, src, `role(?n, "source") => ?n`); col(rows, "n") != "a" {
		t.Errorf("a legal constant = %v, want a", rows)
	}
}

func TestFilterNeedsBoundArguments(t *testing.T) {
	if rows := eval(t, graph(), `node(?n), str.contains(?n, "x") => ?n`); col(rows, "n") != "x" {
		t.Errorf("contains = %v, want x", rows)
	}
	err := evalErr(graph(), `str.contains(?n, "x") => ?n`)
	if err == nil || !strings.Contains(err.Error(), "needs all arguments bound") {
		t.Errorf("err = %v, want the unbound-filter error", err)
	}
}

// succ is a host generator over the Source it is handed: succ(?a, ?b) enumerates edges out of ?a
// (or out of every node when ?a is unbound). It exercises the path a host's own graph walk takes.
func succRegistry() *ns.Vocabulary {
	r := std(graph())
	err := r.AddPredicate("succ", ns.Builtin{Arity: 2, Modes: [][]bool{{true, false}, {false, false}}, Gen: func(_ context.Context, src ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
		for _, t := range src.Tuples("edge") {
			if args[0].Bound && t.Vals[0].S != args[0].Value.S {
				continue
			}
			if err := emit(t.Vals, []string{"succ from " + t.Vals[0].S}); err != nil {
				return err
			}
		}
		return nil
	}})
	if err != nil {
		panic(err)
	}
	return r
}

func TestGeneratorBindsNegatesAndCites(t *testing.T) {
	b := baseFor(succRegistry())
	rows, err := Naive{}.Eval(bg, mustParse(t, `succ("b", ?x) => ?x`), b)
	if err != nil || col(rows, "x") != "c" || len(rows[0].Cites) != 1 {
		t.Fatalf("succ(b) = %v, %v; want c with one citation", rows, err)
	}
	rows, err = Naive{}.Eval(bg, mustParse(t, `node(?n), not succ(?n, ?_) => ?n`), b)
	if err != nil || col(rows, "n") != "d,x" {
		t.Errorf("not succ = %v, %v; want d,x", rows, err)
	}
	// A bound second argument the generator disagrees with is dropped by unification.
	rows, err = Naive{}.Eval(bg, mustParse(t, `succ("a", "c")`), b)
	if err != nil || len(rows) != 0 {
		t.Errorf("succ(a, c) = %v, %v; want nothing", rows, err)
	}
}

// big is a relation past IndexMinTuples whose numbers are stated canonically, so a probe spelled
// 20.0 has to find the fact filed as 20. The indexed and unindexed bases must agree on it.
func big() *ns.MemSource {
	src := ns.NewMemSource().Declare("v", "k", "n")
	for i := 0; i < IndexMinTuples+8; i++ {
		src.Add("v", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("k%d", i)), ns.N(float64(i))}})
	}
	return src
}

func TestIndexedAndUnindexedAgree(t *testing.T) {
	b := baseFor(std(big()))
	for _, q := range []string{`v(?k, 20) => ?k`, `v(?k, 20.0) => ?k`, `v("k3", ?n) => ?n`, `v(?k, ?n), v(?k2, ?n) => ?k, ?k2`} {
		qq := mustParse(t, q)
		indexed, err1 := Naive{}.Eval(bg, qq, b)
		plain, err2 := Naive{}.Eval(bg, qq, b.Unindexed())
		if err1 != nil || err2 != nil {
			t.Fatalf("%s: %v / %v", q, err1, err2)
		}
		if fmt.Sprint(len(indexed)) != fmt.Sprint(len(plain)) || col(indexed, "k") != col(plain, "k") {
			t.Errorf("%s: indexed %v, unindexed %v", q, indexed, plain)
		}
	}
	if rows, _ := (Naive{}).Eval(bg, mustParse(t, `v(?k, 20.0) => ?k`), b); col(rows, "k") != "k20" {
		t.Errorf("20.0 found %v, want k20", rows)
	}
}

func TestRulesDoNotLeakBetweenQueriesOnOneBase(t *testing.T) {
	b := baseFor(std(graph()))
	if _, err := (Naive{}).Eval(bg, mustParse(t, `src(?n) :- edge(?n, ?_); src(?n)`), b); err != nil {
		t.Fatal(err)
	}
	_, err := Naive{}.Eval(bg, mustParse(t, `src(?n)`), b)
	if err == nil || !strings.Contains(err.Error(), "unknown relation") {
		t.Errorf("err = %v, want src to be unknown to the second query", err)
	}
}

func TestRuleHeadCannotRedefineARelationOrPredicate(t *testing.T) {
	for _, q := range []string{`edge(?a, ?b) :- node(?a), node(?b); edge(?a, ?b)`, `absent(?a) :- node(?a); absent(?a)`} {
		if err := evalErr(graph(), q); err == nil || !strings.Contains(err.Error(), "redefines") {
			t.Errorf("%s: err = %v, want a redefinition error", q, err)
		}
	}
}

func TestValidateCatchesALaterAtomWithoutEvaluating(t *testing.T) {
	// Evaluation would stop at the empty first atom and never reach the wrong arity in the second.
	err := Validate(mustParse(t, `edge(?a, "zzz"), node(?a, ?b)`), std(graph()))
	if err == nil || !strings.Contains(err.Error(), "takes 1 args") {
		t.Errorf("err = %v, want the arity error in position two", err)
	}
	// With no vocabulary, only the checks that need one stand down.
	if err := Validate(mustParse(t, `nope(?a)`), std(ns.NewMemSource())); err != nil {
		t.Errorf("err = %v, want no vocabulary check against an empty source", err)
	}
	err = Validate(mustParse(t, `p(?x) :- nope(?x), not q(?x); q(?x) :- nope(?x), not p(?x); p(?x)`), ns.MustVocabulary(ns.NewMemSource()))
	if err == nil || !strings.Contains(err.Error(), "not stratifiable") {
		t.Errorf("err = %v, want stratification still checked with no vocabulary", err)
	}
}

func TestReadsNamesOnlySourceRelations(t *testing.T) {
	q := mustParse(t, `r(?x) :- edge(?x, ?_); r(?x), node(?x), str.contains(?x, "a")`)
	if got := strings.Join(Reads(q, std(graph())), ","); got != "edge,node" {
		t.Errorf("Reads = %s, want edge,node", got)
	}
}

func TestAddPredicateRefusesAndCloneIsIndependent(t *testing.T) {
	r := std(graph())
	if err := r.AddPredicate("str.contains", ns.Filter(2, func([]ns.Value) (bool, error) { return true, nil })); err == nil {
		t.Error("adding str.contains twice was accepted")
	}
	c := r.Clone()
	if err := c.AddPredicate("extra", ns.Filter(1, func([]ns.Value) (bool, error) { return true, nil })); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Predicate("extra"); ok {
		t.Error("a predicate added to the clone reached the original")
	}
	if _, ok := c.Predicate("extra"); !ok {
		t.Error("the clone lost the predicate added to it")
	}
}
