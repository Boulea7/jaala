package datalog

import (
	"github.com/panyam/jaala/ns"
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
func graph() *ns.MemSource {
	src := ns.NewMemSource().
		Declare("edge", "from", "to").
		Declare("node", "name").
		Declare("weight", "node", "w")
	for _, e := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}} {
		src.Add("edge", ns.Tuple{Vals: []ns.Value{ns.S(e[0]), ns.S(e[1])}, Cites: []string{"edge:" + e[0] + e[1]}})
	}
	for i, n := range []string{"a", "b", "c", "d", "x"} {
		src.Add("node", ns.Tuple{Vals: []ns.Value{ns.S(n)}, Cites: []string{"node:" + n}})
		src.Add("weight", ns.Tuple{Vals: []ns.Value{ns.S(n), ns.N(float64(i + 1))}})
	}
	return src
}

// std is src's relations as a vocabulary, with the standard predicates and Datalog modules, the
// vocabulary most tests query. It remembers src, so baseFor can bind the two again.
func std(src ns.Source) *ns.Vocabulary {
	v := ns.MustVocabulary(src)
	if err := v.AddLanguage(Language); err != nil {
		panic(err)
	}
	if err := ns.StandardPredicates(v); err != nil {
		panic(err)
	}
	sources[v] = src
	return v
}

// sources is the Source each test vocabulary was built from. Tests register fixtures sequentially,
// before any goroutine they start, so it needs no lock.
var sources = map[*ns.Vocabulary]ns.Source{}

// baseFor binds a vocabulary std built to the Source it was built from.
func baseFor(v *ns.Vocabulary) *Base { return MustBase(v, sources[v]) }

func eval(t *testing.T, src ns.Source, text string) []Row {
	t.Helper()
	rows, err := Naive{}.Eval(mustParse(t, text), baseFor(std(src)))
	if err != nil {
		t.Fatalf("Eval(%q): %v", text, err)
	}
	return rows
}

func evalErr(src ns.Source, text string) error {
	q, err := Parse(text)
	if err != nil {
		return err
	}
	_, err = Naive{}.Eval(q, baseFor(std(src)))
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
