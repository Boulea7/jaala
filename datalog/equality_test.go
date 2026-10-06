package datalog

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

// equalityMatrix holds text, numbers spelled canonically and not, a unit, absent, the empty text, -0
// and NaN: the values whose equality the old rule got non-transitive.
func equalityMatrix() []ns.Value {
	nan := math.NaN()
	return []ns.Value{
		ns.S("0"), ns.N(0), spelled("00", 0, ""), ns.S("00"), ns.N(math.Copysign(0, -1)), ns.S("-0"),
		ns.S("1"), ns.N(1), spelled("1.0", 1, ""), ns.S("1.0"), spelled("01", 1, ""), ns.S("01"),
		spelled("3.3V", 3.3, "V"), ns.S("3.3V"), ns.S("3.3"), ns.NU(3.3, "V"),
		absentV(), ns.S(""), {S: "NaN", Num: &nan}, ns.S("NaN"), ns.S("word"),
	}
}

func show(v ns.Value) string {
	switch {
	case v.Absent:
		return "absent"
	case v.Num != nil:
		return fmt.Sprintf("number %q", v.S)
	}
	return fmt.Sprintf("text %q", v.S)
}

// Equality is an equivalence (#162), and two values are equal exactly when their keys are, which the
// index relies on: "0", 0 and 00 are one value, and "00" is only itself.
func TestEqualityIsAnEquivalence(t *testing.T) {
	m := equalityMatrix()
	for _, a := range m {
		if !valueEq(a, a) {
			t.Errorf("%s does not equal itself", show(a))
		}
		for _, b := range m {
			if valueEq(a, b) != valueEq(b, a) {
				t.Errorf("%s = %s is not symmetric", show(a), show(b))
			}
			if valueEq(a, b) != (valueKey(a) == valueKey(b)) {
				t.Errorf("%s = %s is %v, but their keys are %q and %q", show(a), show(b), valueEq(a, b), valueKey(a), valueKey(b))
			}
			for _, c := range m {
				if valueEq(a, b) && valueEq(b, c) && !valueEq(a, c) {
					t.Errorf("%s = %s and %s = %s, but %s != %s", show(a), show(b), show(b), show(c), show(a), show(c))
				}
			}
		}
	}
	for _, c := range []struct {
		a, b ns.Value
		eq   bool
	}{
		{ns.N(0), spelled("00", 0, ""), true}, {spelled("1.0", 1, ""), ns.N(1), true}, {ns.NU(3.3, "V"), spelled("3.3V", 3.3, "V"), true},
		{ns.S("0"), ns.N(0), false}, {ns.S("1"), spelled("1.0", 1, ""), false}, {ns.S("3.3V"), spelled("3.3V", 3.3, "V"), false},
		{absentV(), ns.S(""), false}, {ns.N(math.Copysign(0, -1)), ns.N(0), true},
	} {
		if valueEq(c.a, c.b) != c.eq {
			t.Errorf("%s = %s: %v, want %v", show(c.a), show(c.b), valueEq(c.a, c.b), c.eq)
		}
	}
}

// #162's repro: which of z("0") and z(0) is derived first no longer decides whether z(00) holds.
func TestEqualValuesAnswerAlikeInAnyRuleOrder(t *testing.T) {
	v := fuzzVocabulary(t)
	for _, text := range []string{
		`z("0") :- node(?a); z(0) :- weight(?x, ?b); z(00)`,
		`z(0) :- weight(?x, ?b); z("0") :- node(?a); z(00)`,
	} {
		rows, diff, err := agree(mustParse(t, text), baseFor(v))
		if err != nil || diff != "" || len(rows) != 1 {
			t.Errorf("%s: %d rows, %v\n%s", text, len(rows), err, diff)
		}
	}
}

// A join over "0", 0 and 00 answers alike in every literal order: in untyped arguments the text never
// equals the numbers, so no order joins it; with the argument typed a number, the Source's "0" is read
// as 0 and every order joins all three.
func TestAJoinOverTextAndNumbersAnswersAlikeInEveryOrder(t *testing.T) {
	for _, c := range []struct {
		typ  ns.ArgType
		want int
	}{{ns.ArgType{}, 0}, {ns.ArgType{Type: ns.TypeNumber}, 1}} {
		src := ns.NewMemSource()
		for _, rel := range []string{"p", "q", "r"} {
			src.DeclareSchema(rel, ns.Schema{Arity: 1, Labels: []string{"x"}, Types: []ns.ArgType{c.typ}})
		}
		src.Add("p", ns.Tuple{Vals: []ns.Value{ns.S("0")}})
		src.Add("q", ns.Tuple{Vals: []ns.Value{ns.N(0)}})
		src.Add("r", ns.Tuple{Vals: []ns.Value{spelled("00", 0, "")}})
		for _, order := range [][3]string{{"p", "q", "r"}, {"p", "r", "q"}, {"q", "p", "r"}, {"q", "r", "p"}, {"r", "p", "q"}, {"r", "q", "p"}} {
			body := order[0] + "(?x), " + order[1] + "(?x), " + order[2] + "(?x) => ?x"
			if rows := eval(t, src, body); len(rows) != c.want {
				t.Errorf("%q typed %v: %s: %d rows, want %d", c.typ.Type, c.typ, body, len(rows), c.want)
			}
		}
	}
}

// A Source's values are read as their arguments' types when the Base reads them, whole or through
// Lookup: text in a number argument as its number, a number in an entity argument as text, and every
// plain number canonically. Text saying more than its number, and text a number argument can't read,
// stay as given, and the Source's own tuples are never written.
func TestASourceIsReadAsItsDeclaredTypes(t *testing.T) {
	src := ns.NewMemSource().DeclareSchema("part", ns.Schema{Arity: 4, Labels: []string{"ref", "pin", "count", "note"},
		Types: []ns.ArgType{{Kind: "component"}, {Kind: "pin"}, {Type: ns.TypeNumber}, {}}})
	src.Add("part", ns.Tuple{Vals: []ns.Value{ns.S("U1"), ns.N(3), ns.S("0010"), spelled("1.50", 1.5, "")}})
	src.Add("part", ns.Tuple{Vals: []ns.Value{ns.S("U2"), ns.S("A1"), ns.S("n/a"), spelled("3.3V", 3.3, "V")}})
	v := std(src)
	for _, b := range []*Base{baseFor(v), MustBase(v, lookingAt(src))} {
		for _, c := range []struct{ query, col, want string }{
			{`part(?r, "3", _, _) => ?r`, "r", "U1"},
			{`part(?r, 3, _, _) => ?r`, "r", "U1"},
			{`part(?r, _, 10, _) => ?r`, "r", "U1"},
			{`part("U1", _, ?c, _) => ?c`, "c", "10"},
			{`part("U1", _, _, ?n) => ?n`, "n", "1.5"},
			{`part("U2", _, ?c, ?n) => ?c, ?n`, "c", "n/a"},
			{`part("U2", _, _, ?n) => ?n`, "n", "3.3V"},
			// While the query runs, not only in its answer: a string filter reads 1.5, not 1.50.
			{`part(?r, _, _, ?n), str.contains(?n, "50") => ?r`, "r", ""},
			{`part(?r, _, _, ?n), str.contains(?n, ".5") => ?r`, "r", "U1"},
		} {
			rows, err := both(mustParse(t, c.query), b)
			if err != nil || col(rows, c.col) != c.want {
				t.Errorf("%T %s: %q, %v; want %q", b.src, c.query, col(rows, c.col), err, c.want)
			}
		}
		rows, _ := both(mustParse(t, `part("U1", ?p, ?c, _) => ?p, ?c`), b)
		if len(rows) != 1 || rows[0].Bind["p"].Num != nil || rows[0].Bind["c"].Num == nil {
			t.Errorf("%T: the pin should read as text and the count as a number: %v", b.src, rows)
		}
	}
	if got := src.Tuples("part")[0].Vals; got[1].Num == nil || got[2].S != "0010" {
		t.Errorf("the Source's tuple was written: %v", got)
	}
}

// The index finds exactly what a scan does for every value of the matrix, on a relation big enough to
// index. control: Unindexed scans.
func TestTheIndexFindsWhatAScanFinds(t *testing.T) {
	src := ns.NewMemSource().Declare("w", "k", "v")
	m := equalityMatrix()
	for i := 0; i < 2*IndexMinTuples; i++ {
		src.Add("w", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("k%d", i)), m[i%len(m)]}})
	}
	b := baseFor(std(src))
	for _, probe := range m {
		q := Query{Goal: Body{Literals: []Literal{{Pos: &Atom{Relation: "w", Args: []Term{{Var: "k"}, {Const: &probe}}}}}}, Select: []Term{{Var: "k"}}}
		var got [2]string
		for i, base := range []*Base{b.Unindexed(), b} {
			rows, err := (SemiNaive{}).Eval(bg, q, base)
			if err != nil {
				t.Fatal(err)
			}
			got[i] = col(rows, "k")
		}
		if got[0] != got[1] {
			t.Errorf("probing %s: scan %q, index %q", show(probe), got[0], got[1])
		}
	}
}

// A variable's name is an ident (#163), wherever a variable is written, and a number is spelled as
// the grammar says. control: the names and numbers the grammar allows still parse.
func TestAVariableIsAnIdentAndANumberIsDecimal(t *testing.T) {
	for _, text := range []string{
		`weight(?)(, ?b) => ?b`, `node(?a.b)`, `node(?a-b)`, `node(? x)`, `node(?x) => ?a.b`, `node(?x) => count(?a-b)`,
		`r(?a.b: number) :- node(?a.b); r(?x)`,
	} {
		_, err := Parse(text)
		if err == nil || !strings.HasPrefix(err.Error(), "query: ") || !strings.Contains(err.Error(), "a variable's name is letters, digits and _") {
			t.Errorf("%s: %v; want the variable refused", text, err)
		}
	}
	for _, num := range []string{"inf", "-Inf", "nan", "NaN", "0x1p-2", ".5", "5.", "1_000", "1e", "+"} {
		if _, err := Parse(`weight(?n, ` + num + `)`); err == nil || !strings.Contains(err.Error(), "bare identifier") {
			t.Errorf("%s: %v; want it refused as a bare identifier", num, err)
		}
	}
	for _, text := range []string{`weight(?x_7, ?_a), node(?0)`, `weight(?n, 1e-9)`, `weight(?n, -2.5)`, `weight(?n, +3)`, `weight(?n, 1E6)`, `weight(?n, 007)`} {
		if _, err := Parse(text); err != nil {
			t.Errorf("control: %s: %v", text, err)
		}
	}
}
