package datalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

// citesOf is each row's citations, keyed by col's value.
func citesOf(rows []Row, col Var) map[string]string {
	out := map[string]string{}
	for _, r := range rows {
		out[r.Bind[col].S] = strings.Join(r.Cites, " ")
	}
	return out
}

// edges is a source of edge facts, stored in the order given and cited as edge:from-to, with the
// nodes they name.
func edges(pairs ...string) *ns.MemSource {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name")
	seen := map[string]bool{}
	for _, p := range pairs {
		from, to, _ := strings.Cut(p, "-")
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(from), ns.S(to)}, Cites: []string{"edge:" + p}})
		for _, n := range []string{from, to} {
			if !seen[n] {
				seen[n] = true
				src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(n)}})
			}
		}
	}
	return src
}

// Under CanonicalCites a tuple keeps its shortest derivation, from every evaluator (#22). Here v2 is
// in r2 by one step from an edge into it, and by two through r3. control: without the option the two
// fixpoints keep different derivations, so the fixture can tell.
func TestCanonicalCitesKeepTheShortestDerivation(t *testing.T) {
	b := baseFor(std(edges("2-1", "3-2")))
	q := mustParse(t, `r2(?a, ?a) :- r3(?a); r2(?b, ?b) :- edge(?c, ?b); r3(?d) :- edge(?d, ?e); r2(?x, ?y) => ?y`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, q, b, CanonicalCites())
		if got := citesOf(rows, "y")["2"]; err != nil || got != "edge:3-2" {
			t.Errorf("%T: v2 cites %q, %v; want edge:3-2, the one-step derivation", ev, got, err)
		}
	}
	naive, _ := Naive{}.Eval(bg, q, b)
	semi, _ := SemiNaive{WrittenOrder: true}.Eval(bg, q, b)
	if citesOf(naive, "y")["2"] == citesOf(semi, "y")["2"] {
		t.Errorf("control: without CanonicalCites both cite %q for v2, so this fixture cannot show a choice", citesOf(naive, "y")["2"])
	}
}

// Of two derivations equally short, the one whose body facts come first by value is kept: reach(a, d)
// through b, not c, though the edge through c is stored first. Rows carry no witness unless Witnesses
// asks, and with it the witness is the derivation the citations come from. control: without the
// option, the first edge stored is the one cited.
func TestCanonicalCitesBreakATieByValue(t *testing.T) {
	b := baseFor(std(edges("a-c", "c-d", "a-b", "b-d")))
	q := mustParse(t, leftReach+`reach("a", ?x) => ?x`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, q, b, CanonicalCites())
		if got := citesOf(rows, "x")["d"]; err != nil || got != "edge:a-b edge:b-d" {
			t.Errorf("%T: reach(a, d) cites %q, %v; want edge:a-b edge:b-d", ev, got, err)
		}
		for _, r := range rows {
			if r.Witness != nil {
				t.Errorf("%T: row %v has a witness, but Witnesses wasn't asked for", ev, r.Bind)
			}
		}
		rows, err = ev.Eval(bg, q, b, CanonicalCites(), Witnesses())
		for _, r := range rows {
			if r.Bind["x"].S == "d" && (err != nil || fmt.Sprint(r.Witness[0].Children[0].Values) != fmt.Sprint([]ns.Value{ns.S("a"), ns.S("b")})) {
				t.Errorf("%T: reach(a, d)'s witness reads %v, %v; want reach(a, b) first", ev, r.Witness[0].Children[0].Values, err)
			}
		}
	}
	plain, _ := Naive{}.Eval(bg, q, b)
	if got := citesOf(plain, "x")["d"]; got != "edge:a-c edge:c-d" {
		t.Errorf("control: without CanonicalCites reach(a, d) cites %q; want the first stored, edge:a-c edge:c-d", got)
	}
}

// A tuple a later round derives more shortly is derived from again, so what was derived from it
// shortens too. b(v9) is first found through the a-chain (ten steps), then in one hop from b(v1); b(v10)
// comes from b(v9), so it must follow the hop once b(v9) does. Without passing the shorter b(v9) on,
// b(v10) keeps its first derivation through the chain. control: without the option it does.
func TestACanonicalDerivationThatShortensIsPassedOn(t *testing.T) {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("hop", "from", "to")
	for i := 0; i < 10; i++ {
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", i)), ns.S(fmt.Sprintf("v%d", i+1))}, Cites: []string{fmt.Sprintf("edge:%d", i)}})
	}
	for _, h := range [][2]int{{1, 9}, {9, 10}} {
		src.Add("hop", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", h[0])), ns.S(fmt.Sprintf("v%d", h[1]))}, Cites: []string{fmt.Sprintf("hop:%d-%d", h[0], h[1])}})
	}
	b := baseFor(std(src))
	q := mustParse(t, `a(?x, ?y) :- edge(?x, ?y); a(?x, ?z) :- a(?x, ?y), edge(?y, ?z); `+
		`b(?x) :- a("v0", ?x); b(?y) :- b(?x), hop(?x, ?y); b(?x) => ?x`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, q, b, CanonicalCites())
		cites := citesOf(rows, "x")
		if err != nil || cites["v9"] != "edge:0 hop:1-9" || cites["v10"] != "edge:0 hop:1-9 hop:9-10" {
			t.Errorf("%T: b(v9) cites %q, b(v10) %q, %v; want edge:0 hop:1-9, and edge:0 hop:1-9 hop:9-10", ev, cites["v9"], cites["v10"], err)
		}
	}
	plain, _ := SemiNaive{WrittenOrder: true}.Eval(bg, q, b)
	if got := citesOf(plain, "x")["v10"]; strings.Contains(got, "hop") {
		t.Errorf("control: without CanonicalCites b(v10) cites %q; want the chain it was first found through", got)
	}
}

// An answer row that several bindings project to keeps the first binding's citations by the same
// order: d is reached from b and from c, and cites the edge from b. control: without the option, the
// first edge stored.
func TestACanonicalRowKeepsItsFirstBinding(t *testing.T) {
	b := baseFor(std(edges("c-d", "b-d")))
	q := mustParse(t, `edge(?x, ?y) => ?y`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, q, b, CanonicalCites())
		if got := citesOf(rows, "y")["d"]; err != nil || got != "edge:b-d" {
			t.Errorf("%T: d cites %q, %v; want edge:b-d", ev, got, err)
		}
	}
	if plain, _ := (Naive{}).Eval(bg, q, b); citesOf(plain, "y")["d"] != "edge:c-d" {
		t.Errorf("control: without CanonicalCites d cites %q; want the first stored, edge:c-d", citesOf(plain, "y")["d"])
	}
}

// A body prefix the demand rewrite would store in a supplementary relation is a set over its named
// variables, so it merges derivations that differ only in a `_`; under CanonicalCites the prefix is
// not stored, and the aggregate's citations are the union Naive gives. The generated corpus found
// this (seed 2726, shrunk). control: SemiNaive with no rewrites gives the same citations, so the
// difference would be the rewrite's.
func TestCanonicalCitesThroughADemandedPrefix(t *testing.T) {
	src := ns.NewMemSource().Declare("edge", "from", "to").Declare("node", "name").Declare("weight", "node", "w")
	for i, w := range []int{3, 1, 7, 7, 2, 0, 9, 3} {
		n := fmt.Sprintf("v%d", i)
		src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(n)}, Cites: []string{"node:" + n[1:]}})
		src.Add("weight", ns.Tuple{Vals: []ns.Value{ns.S(n), ns.N(float64(w))}})
	}
	for _, e := range [][2]int{{6, 1}, {6, 6}, {7, 6}} {
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", e[0])), ns.S(fmt.Sprintf("v%d", e[1]))}, Cites: []string{fmt.Sprintf("edge:%d-%d", e[0], e[1])}})
	}
	b := baseFor(std(src))
	q := mustParse(t, `r1(?x9, ?x7, ?x8) :- edge(?x6, ?x7), edge(?x7, ?x8), weight(?x6, ?x9); r1(?x10, ?x11, ?x11) :- weight(_, ?x10), node(?x11); `+
		`r1(_, ?x17, ?x18), r1(?x19, ?x17, ?x17) => max(?x19)`)
	want, err := SemiNaive{WrittenOrder: true}.Eval(bg, q, b, CanonicalCites())
	if err != nil || len(want) != 1 || !strings.Contains(strings.Join(want[0].Cites, " "), "edge:6-6") {
		t.Fatalf("control: written order cites %v, %v; want one row citing edge:6-6", want, err)
	}
	got, err := SemiNaive{}.Eval(bg, q, b, CanonicalCites())
	if err != nil || len(got) != 1 || strings.Join(got[0].Cites, " ") != strings.Join(want[0].Cites, " ") {
		t.Errorf("planned cites %v, %v; want %v", got, err, want[0].Cites)
	}
}

// A tuple derived from one that is later kept another way, equally short, takes up its new citations.
// b(t) is three steps through a(t) and g, and three through b(s), whose rule sorts first. SemiNaive
// finds b(t) through a(t) first and derives b(u) from it before b(s) comes up, so b(u) has to be
// derived again to cite the new path, though the old one's citations sort first. control: without the option, SemiNaive's b(u) cites the path
// through a(t).
func TestACanonicalTupleFollowsItsChildsNewEvidence(t *testing.T) {
	src := ns.NewMemSource().Declare("e0", "x").Declare("f", "from", "to").Declare("e", "from", "to")
	src.Add("e0", ns.Tuple{Vals: []ns.Value{ns.S("s")}, Cites: []string{"e0:s"}})
	src.Add("e0", ns.Tuple{Vals: []ns.Value{ns.S("t0")}, Cites: []string{"a0:t0"}})
	src.Add("f", ns.Tuple{Vals: []ns.Value{ns.S("t0"), ns.S("t")}, Cites: []string{"a1:t0-t"}})
	src.Add("e", ns.Tuple{Vals: []ns.Value{ns.S("s"), ns.S("t")}, Cites: []string{"e:s-t"}})
	src.Add("e", ns.Tuple{Vals: []ns.Value{ns.S("t"), ns.S("u")}, Cites: []string{"e:t-u"}})
	b := baseFor(std(src))
	q := mustParse(t, `g(?x) :- e0(?y), f(?y, ?x); a(?x) :- g(?x); a(?x) :- e0(?x); `+
		`b(?x) :- a(?x); b(?a) :- b(?c), e(?c, ?a); b(?x) => ?x`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, q, b, CanonicalCites())
		if got := citesOf(rows, "x")["u"]; err != nil || got != "e0:s e:s-t e:t-u" {
			t.Errorf("%T: b(u) cites %q, %v; want e0:s e:s-t e:t-u", ev, got, err)
		}
	}
	if plain, _ := (SemiNaive{WrittenOrder: true}).Eval(bg, q, b); citesOf(plain, "x")["u"] != "a0:t0 a1:t0-t e:t-u" {
		t.Errorf("control: SemiNaive without CanonicalCites cites %q for u; want the path through a(t)", citesOf(plain, "x")["u"])
	}
}

// Naive stops after a round that adds nothing, so a derivation a round replaces counts as a change: it
// runs again, and what read the replaced one is derived anew. Relations run in name order, so b(t) is
// found through ab, a(u) from it, and b(t)'s other derivation, as short and by a rule that sorts first
// (through c), only in the round after c is, which adds nothing new. control: with Naive stopping
// there, a(u)'s witness would read b(t) through ab, which SemiNaive never gives.
func TestNaiveRunsAgainWhenARoundOnlyReplacesADerivation(t *testing.T) {
	src := ns.NewMemSource().Declare("e", "x").Declare("f", "from", "to")
	src.Add("e", ns.Tuple{Vals: []ns.Value{ns.S("t")}, Cites: []string{"e:t"}})
	src.Add("f", ns.Tuple{Vals: []ns.Value{ns.S("t"), ns.S("u")}, Cites: []string{"f:t-u"}})
	b := baseFor(std(src))
	q := mustParse(t, `aa(?x) :- e(?x); ab(?x) :- aa(?x); b(?x) :- ab(?x); b(?a) :- c(?a); c(?x) :- d(?x); d(?x) :- e(?x); `+
		`a(?u) :- b(?t), f(?t, ?u); a(?u) :- a(?u), e(?u); a(?x) => ?x`)
	rows, err := both(q, b, CanonicalCites(), Witnesses())
	if err != nil || len(rows) != 1 || rows[0].Witness[0].Children[0].Rule != "b(?a) :- c(?a)" {
		t.Fatalf("a(u): %v, %v; want one row whose b(t) comes through c", rows, err)
	}
	for _, ev := range evaluators {
		rows, _ := ev.Eval(bg, q, b, CanonicalCites(), Witnesses())
		if got := rows[0].Witness[0].Children[0].Rule; got != "b(?a) :- c(?a)" {
			t.Errorf("%T: a(u) reads b(t) by %q; want b(?a) :- c(?a)", ev, got)
		}
	}
}
