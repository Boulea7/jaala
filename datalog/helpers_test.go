package datalog

import (
	"sort"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) Query {
	t.Helper()
	q, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return q
}

// graph is a small directed graph with a string label per node, the fixture most engine tests run
// over. Edges: a->b, b->c, c->d, and x standing alone. Labels are numeric on purpose, so comparisons
// and aggregates have something to order.
func graph() *MemSource {
	src := NewMemSource().
		Declare("edge", "from", "to").
		Declare("node", "name").
		Declare("weight", "node", "w")
	for _, e := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}} {
		src.Add("edge", Tuple{Vals: []Value{S(e[0]), S(e[1])}, Cites: []string{"edge:" + e[0] + e[1]}})
	}
	for i, n := range []string{"a", "b", "c", "d", "x"} {
		src.Add("node", Tuple{Vals: []Value{S(n)}, Cites: []string{"node:" + n}})
		src.Add("weight", Tuple{Vals: []Value{S(n), N(float64(i + 1))}})
	}
	return src
}

func eval(t *testing.T, src Source, text string) []Row {
	t.Helper()
	rows, err := Naive{}.Eval(mustParse(t, text), NewBase(src, StandardPredicates()))
	if err != nil {
		t.Fatalf("Eval(%q): %v", text, err)
	}
	return rows
}

func evalErr(src Source, text string) error {
	q, err := Parse(text)
	if err != nil {
		return err
	}
	_, err = Naive{}.Eval(q, NewBase(src, StandardPredicates()))
	return err
}

// col joins one column of the answer, sorted, so a test compares a whole result in one string.
func col(rows []Row, v string) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Bind[Var(v)].S)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
