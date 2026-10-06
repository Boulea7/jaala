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
		{ns.S("0"), spelled("00", 0, ""), true}, {ns.S("00"), ns.N(0), false}, {ns.S("1"), spelled("1.0", 1, ""), true},
		{ns.S("3.3V"), spelled("3.3V", 3.3, "V"), false}, {ns.S("3.3"), spelled("3.3V", 3.3, "V"), true},
		{absentV(), ns.S(""), false}, {ns.S("-0"), ns.N(0), false},
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

// A join over "0", 0 and 00 holds in every literal order, in every evaluator.
func TestAJoinOverTextAndNumbersHoldsInEveryOrder(t *testing.T) {
	src := ns.NewMemSource().Declare("p", "x").Declare("q", "x").Declare("r", "x")
	src.Add("p", ns.Tuple{Vals: []ns.Value{ns.S("0")}})
	src.Add("q", ns.Tuple{Vals: []ns.Value{ns.N(0)}})
	src.Add("r", ns.Tuple{Vals: []ns.Value{spelled("00", 0, "")}})
	for _, order := range [][3]string{{"p", "q", "r"}, {"p", "r", "q"}, {"q", "p", "r"}, {"q", "r", "p"}, {"r", "p", "q"}, {"r", "q", "p"}} {
		body := order[0] + "(?x), " + order[1] + "(?x), " + order[2] + "(?x) => ?x"
		if rows := eval(t, src, body); len(rows) != 1 {
			t.Errorf("%s: %d rows, want 1", body, len(rows))
		}
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
