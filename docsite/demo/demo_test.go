package demo

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseFacts(t *testing.T) {
	facts, err := ParseFacts(`# a comment
edge("a", "b"); edge("b;c", "#d").
weight("a", 1.5)`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range facts {
		got = append(got, f.Text)
	}
	want := `edge("a", "b") | edge("b;c", "#d") | weight("a", 1.5)`
	if strings.Join(got, " | ") != want {
		t.Errorf("parsed %q, want %q", strings.Join(got, " | "), want)
	}
	if facts[2].Args[1].Num == nil || *facts[2].Args[1].Num != 1.5 {
		t.Errorf("weight's second argument should be the number 1.5: %+v", facts[2].Args[1])
	}
	for _, bad := range []string{`edge("a", b)`, `edge "a"`, `ok("a")` + "\n" + `(`} {
		if _, err := ParseFacts(bad); err == nil || !strings.Contains(err.Error(), "facts line") {
			t.Errorf("ParseFacts(%q) = %v, want an error naming its line", bad, err)
		}
	}
}

func TestRun(t *testing.T) {
	tab, err := Run(Spec{Fixture: "graph", Program: `out(?n, count(distinct ?m)) :- edge(?n, ?m);`, Query: `out(?n, ?c), weight(?n, ?w) => ?n, ?c, sum(?w)`})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tab.Columns, " "); got != "?n ?c sum(?w)" {
		t.Errorf("columns %q, want the query's own spelling", got)
	}
	tab, err = Run(Spec{Facts: `node("a"); node("b")`, Query: `node(?n), not node("z") => ?n`, Bind: map[string]string{"n": "b"}})
	if err != nil || len(tab.Rows) != 1 || tab.Rows[0][0] != "b" {
		t.Errorf("a bound ?n: %v, %v; want the one row b", tab.Rows, err)
	}
	if _, err := Run(Spec{Facts: `e("a"); e("a", "b")`, Query: `e(?x) => ?x`}); err == nil || !strings.Contains(err.Error(), "arguments") {
		t.Errorf("a relation with two arities: %v, want refused", err)
	}
	if _, err := Run(Spec{Fixture: "nope", Query: `x(?a) => ?a`}); err == nil {
		t.Error("an unknown fixture should be refused")
	}
	// A runaway example fails on the budget instead of running on: a four-way cross product over 60
	// facts is 13 million bindings.
	var facts strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&facts, "n(%d)\n", i)
	}
	_, err = Run(Spec{Facts: facts.String(), Program: `p(?a, ?b) :- n(?a), n(?b);`, Query: `p(?a, ?b), p(?c, ?d) => count(?a)`})
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("a runaway example: %v, want it stopped by the budget", err)
	}
}

func TestCheck(t *testing.T) {
	tab := Table{Rows: [][]string{{"b"}}}
	for _, c := range []struct {
		why  string
		spec Spec
		err  error
		ok   bool
	}{
		{"pinned rows match", Spec{Expect: [][]string{{"b"}}}, nil, true},
		{"pinned rows differ", Spec{Expect: [][]string{{"c"}}}, nil, false},
		{"nothing pinned, no error", Spec{}, nil, true},
		{"nothing pinned, an error", Spec{}, errString("query: boom"), false},
		{"pinned error matches", Spec{ExpectError: "boom"}, errString("query: boom"), true},
		{"pinned error differs", Spec{ExpectError: "bang"}, errString("query: boom"), false},
		{"pinned error, none came", Spec{ExpectError: "boom"}, nil, false},
		{"pinned empty answer", Spec{Expect: [][]string{}}, nil, false},
	} {
		if err := Check(c.spec, tab, c.err); (err == nil) != c.ok {
			t.Errorf("%s: Check = %v, want ok=%v", c.why, err, c.ok)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
