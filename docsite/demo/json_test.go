package demo

import (
	"encoding/json"
	"strings"
	"testing"
)

func runJSON(t *testing.T, spec string) Result {
	t.Helper()
	var r Result
	if err := json.Unmarshal([]byte(RunJSON(spec)), &r); err != nil {
		t.Fatalf("RunJSON(%s) didn't answer JSON: %v", spec, err)
	}
	return r
}

// The JSON boundary answers what Run answers: the table for a spec that runs, the query's own error
// for one that doesn't, and an error rather than a panic for a spec that doesn't decode.
func TestRunJSON(t *testing.T) {
	r := runJSON(t, `{"fixture": "graph", "program": "reach(?a, ?b) :- edge(?a, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c);", "query": "reach(\"a\", ?x) => ?x"}`)
	if r.Error != "" || strings.Join(r.Columns, ",") != "?x" || len(r.Rows) != 3 || r.Rows[2][0] != "d" || len(r.Cites[2]) != 3 {
		t.Errorf("reach from a: %+v, want ?x = b, c, d with d citing three edges", r)
	}
	if r := runJSON(t, `{"fixture": "graph", "query": "edge(?x) => ?x"}`); !strings.HasPrefix(r.Error, "query: ") || r.Rows != nil {
		t.Errorf("a wrong arity: %+v, want the query: error and no table", r)
	}
	if r := runJSON(t, `{"query": `); !strings.Contains(r.Error, "isn't valid JSON") {
		t.Errorf("malformed JSON: %+v, want it reported", r)
	}
	if r := runJSON(t, `{"facts": "n(1); n(2); n(3); n(4); n(5); n(6); n(7); n(8); n(9); n(10)", "program": "p(?a, ?b, ?c) :- n(?a), n(?b), n(?c);", "query": "p(?a, ?b, ?c), p(?d, ?e, ?f), p(?g, ?h, ?i) => count(?a)"}`); !strings.Contains(r.Error, "budget") {
		t.Errorf("a runaway edit: %+v, want it stopped by the budget", r)
	}
}
