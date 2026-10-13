package datalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

// ohmsLaw is two resistors: R1 is 1000 ohm carrying 2 mA on a 5 V rail, R2 470 ohm carrying 10 mA on
// a 3.3 V rail.
func ohmsLaw() *ns.MemSource {
	num := func(unit string) []ns.ArgType { return []ns.ArgType{{}, {Type: ns.TypeNumber, Unit: unit}} }
	src := ns.NewMemSource().
		DeclareSchema("res", ns.Schema{Arity: 2, Labels: []string{"part", "r"}, Types: num("ohm")}).
		DeclareSchema("cur", ns.Schema{Arity: 2, Labels: []string{"part", "i"}, Types: num("A")}).
		DeclareSchema("rail", ns.Schema{Arity: 2, Labels: []string{"part", "v"}, Types: num("V")})
	for _, p := range []struct {
		part    string
		r, i, v float64
	}{{"R1", 1000, 0.002, 5}, {"R2", 470, 0.01, 3.3}} {
		src.Add("res", ns.Tuple{Vals: []ns.Value{ns.S(p.part), ns.NU(p.r, "ohm")}}).
			Add("cur", ns.Tuple{Vals: []ns.Value{ns.S(p.part), ns.NU(p.i, "A")}}).
			Add("rail", ns.Tuple{Vals: []ns.Value{ns.S(p.part), ns.NU(p.v, "V")}})
	}
	return src
}

func TestArithmeticComputesAndKeepsUnits(t *testing.T) {
	for _, c := range []struct {
		name, q string
		cols    []string
		want    []string
	}{
		{"a product's unit is named", `res(?p, ?r), cur(?p, ?i), ?v = ?i * ?r => ?p, ?v`, []string{"p", "v"}, []string{"R1 2[V]", "R2 4.7[V]"}},
		{"solved for the missing factor", `rail(?p, ?v), cur(?p, ?i), ?v = ?i * ?r => ?p, ?r`, []string{"p", "r"}, []string{"R1 2500[ohm]", "R2 330[ohm]"}},
		{"written before what binds it", `?v = ?i * ?r, res(?p, ?r), cur(?p, ?i) => ?p, ?v`, []string{"p", "v"}, []string{"R1 2[V]", "R2 4.7[V]"}},
		{"in a comparison", `res(?p, ?r), cur(?p, ?i), rail(?p, ?v), ?i * ?r > ?v / 2 => ?p`, []string{"p"}, []string{"R2"}},
		{"a unit literal folds its prefix", `res(?p, ?r), ?r < 1kohm => ?p`, []string{"p"}, []string{"R2"}},
		{"a sum of one unit", `rail(?p, ?v), ?x = ?v + 1V => ?p, ?x`, []string{"p", "x"}, []string{"R1 6[V]", "R2 4.3[V]"}},
		{"a sum of two units has no result", `rail(?p, ?v), cur(?p, ?i), ?x = ?v + ?i => ?p`, []string{"p"}, nil},
		{"precedence and parentheses", `res(?p, _), ?a = 1 + 2 * 3, ?b = (1 + 2) * 3, ?p = "R1" => ?a, ?b`, []string{"a", "b"}, []string{"7 9"}},
		{"negation and a call", `res(?p, ?r), ?n = -num.add(?r, 10ohm), ?p = "R2" => ?n`, []string{"n"}, []string{"-480[ohm]"}},
		{"division by zero has no result", `res(?p, ?r), ?x = ?r / 0 => ?p`, []string{"p"}, nil},
		{"a quotient nobody named", `res(?p, ?r), ?x = ?r / 2s, ?p = "R1" => ?x`, []string{"x"}, []string{"500[ohm/s]"}},
		{"the same calls bind once", `rail(?p, ?v), cur(?p, ?i), ?v = ?i * ?r, ?r > 1kohm => ?p`, []string{"p"}, []string{"R1"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, extra := range [][]Option{nil, {Witnesses()}} {
				rows, err := both(mustParse(t, c.q), baseFor(std(ohmsLaw())), extra...)
				if err != nil {
					t.Fatal(err)
				}
				if got := withUnits(rows, c.cols...); !reflect.DeepEqual(got, c.want) {
					t.Errorf("%d extra options: %v, want %v", len(extra), got, c.want)
				}
			}
		})
	}
}

func TestArithmeticRefusals(t *testing.T) {
	for _, c := range []struct{ q, want string }{
		{`n(?x) :- res(_, ?x); n(?y) :- n(?x), ?y = ?x + 1; n(?y) => ?y`, `rule "n" computes with arithmetic and reads n, which reaches it again, so it might never finish`},
		{`res(?p, ?r), ?y = ?r + => ?p`, `ends early`},
		{`res(?p, ?r), ?y = (?r + 1 => ?p`, `unclosed (`},
		{`res(?p, ?r), ?y = ?q * 2 => ?p`, `nothing binds what num.mul needs first`},
	} {
		q, err := Parse(c.q)
		if err == nil {
			// A budget, so a recursion the check misses stops rather than hangs.
			_, err = both(q, baseFor(std(ohmsLaw())), Budget(100_000))
		}
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.HasPrefix(err.Error(), "query: ") {
			t.Errorf("%s: err = %v, want it to say %q", c.q, err, c.want)
		}
	}
	// A vocabulary without the standard library's functions says which one arithmetic needs.
	v := ns.MustVocabulary(ohmsLaw())
	if err := v.AddLanguage(Language); err != nil {
		t.Fatal(err)
	}
	if err := Validate(mustParse(t, `res(?p, ?r), ?y = ?r * 2 => ?p`), v); err == nil || !strings.Contains(err.Error(), `arithmetic needs num.mul, which stdlib.Arithmetic registers`) {
		t.Errorf("without stdlib: %v", err)
	}
}

func TestAnExpressionPrintsAsItParses(t *testing.T) {
	for _, text := range []string{
		`res(?p, ?r), ?y = (?r + 1) * -?r, ?z = num.add(?r, 2) / 3, ?r < 3.3V => ?y`,
		`res(?p, ?r), ?y = ?r - (1 - 2) => ?y`,
	} {
		q := mustParse(t, text)
		again, err := Parse(q.Goal.String() + " => ?y")
		if err != nil {
			t.Fatalf("%s printed as %s, which doesn't parse: %v", text, q.Goal, err)
		}
		if !reflect.DeepEqual(again.Goal, q.Goal) {
			t.Errorf("%s printed as %s, which parses differently", text, q.Goal)
		}
	}
}
