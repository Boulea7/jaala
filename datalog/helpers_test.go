package datalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/panyam/jaala/ns"
	"github.com/panyam/jaala/stdlib"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// bg is the context tests evaluate under when they are not testing cancellation.
var bg = context.Background()

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
	if err := stdlib.Register(v); err != nil {
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

// both answers q with the reference evaluator and with SemiNaive, and panics when they disagree, so
// every test that evaluates through these helpers is also a differential test.
//
// SemiNaive in written order must match Naive's rows and errors exactly. Planned, it must give the
// same rows, and Naive's error whenever it errors itself; it may succeed where Naive's written order
// stops on an unbound check, which is what planning is for. A tuple reachable two ways keeps whichever
// derivation an evaluator finds first, so citations are compared under CanonicalCites (#22), where all
// three must give the same rows, citations and witnesses. It returns Naive's answer. opts go to every
// Eval.
func both(q Query, b *Base, opts ...Option) ([]Row, error) {
	rows, diff, err := agree(q, b, opts...)
	if diff != "" {
		panic(diff)
	}
	return rows, err
}

// agree is both without the panic: it returns Naive's answer and, when an evaluator disagrees with
// it, what differed, so the generated corpus can shrink a disagreement instead of stopping on it.
func agree(q Query, b *Base, opts ...Option) ([]Row, string, error) {
	want, werr := Naive{}.Eval(bg, q, b, opts...)
	got, gerr := SemiNaive{WrittenOrder: true}.Eval(bg, q, b, opts...)
	if fmt.Sprint(werr) != fmt.Sprint(gerr) || !reflect.DeepEqual(binds(want), binds(got)) {
		return want, fmt.Sprintf("SemiNaive disagrees with Naive on %v\n naive:     %v %v\n seminaive: %v %v", q, want, werr, got, gerr), werr
	}
	planned, perr := SemiNaive{}.Eval(bg, q, b, opts...)
	switch {
	case perr != nil && fmt.Sprint(perr) != fmt.Sprint(werr):
		return want, fmt.Sprintf("planned SemiNaive fails where Naive does not on %v\n naive:   %v\n planned: %v", q, werr, perr), werr
	case perr == nil && werr == nil && !reflect.DeepEqual(binds(want), binds(planned)):
		return want, fmt.Sprintf("planned SemiNaive answers differently on %v\n naive:   %v\n planned: %v", q, binds(want), binds(planned)), werr
	}
	if werr == nil {
		return want, agreeCanonically(q, b, want, opts), nil
	}
	return want, "", werr
}

// agreeCanonically runs q under CanonicalCites through all three evaluators, which must give the
// rows Naive gave without it, and the same citations and witnesses as one another (#22). A run the
// extra comparisons push past its budget is not compared.
func agreeCanonically(q Query, b *Base, want []Row, opts []Option) string {
	opts = append(opts[:len(opts):len(opts)], CanonicalCites())
	var got [3][]Row
	for i, ev := range []Evaluator{Naive{}, SemiNaive{WrittenOrder: true}, SemiNaive{}} {
		rows, err := ev.Eval(bg, q, b, opts...)
		var over *BudgetExceeded
		if errors.As(err, &over) {
			return ""
		}
		if err != nil {
			return fmt.Sprintf("%T fails under CanonicalCites on %v: %v", ev, q, err)
		}
		got[i] = rows
	}
	if !reflect.DeepEqual(binds(got[0]), binds(want)) {
		return fmt.Sprintf("CanonicalCites changes Naive's rows on %v\n plain:     %v\n canonical: %v", q, binds(want), binds(got[0]))
	}
	for i, name := range []string{"", "SemiNaive", "planned SemiNaive"} {
		if i > 0 && !reflect.DeepEqual(got[0], got[i]) {
			return fmt.Sprintf("%s cites differently from Naive under CanonicalCites on %v\n naive: %v\n %s: %v", name, q, got[0], name, got[i])
		}
	}
	return ""
}

// binds is an answer's rows without their citations.
func binds(rows []Row) []map[Var]ns.Value {
	out := make([]map[Var]ns.Value, len(rows))
	for i, r := range rows {
		out[i] = r.Bind
	}
	return out
}

func eval(t *testing.T, src ns.Source, text string) []Row {
	t.Helper()
	rows, err := both(mustParse(t, text), baseFor(std(src)))
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
	_, err = both(q, baseFor(std(src)))
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
