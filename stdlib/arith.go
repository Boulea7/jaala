package stdlib

import (
	"context"
	"errors"

	"github.com/panyam/jaala/ns"
)

// Arithmetic registers num.add, num.sub, num.mul, num.div and num.neg, the functions a query's
// arithmetic is written in (jaala#5): ?y = ?a * (?b + ?c) runs as num.add(?b, ?c, ?t), num.mul(?a,
// ?t, ?y). Each is a relation of its operands and result, so its modes run it in every direction
// it can: num.mul(?i, ?r, ?v) finds ?v from ?i and ?r, or ?r from ?v and ?i (Ohm's law, whichever is
// missing). Units follow the vocabulary's table: a sum needs one unit (a bare number takes the other
// side's), a product multiplies them, so amps times ohms is V once V is defined as A*ohm. A value
// that isn't a number, a sum of two units, and a division by zero have no result, so the call gives
// no row, as a comparison of unlike units holds for none.
func Arithmetic(v *ns.Vocabulary) error {
	t := func() *ns.Units { return v.Units() }
	for _, p := range []struct {
		path string
		b    ns.Builtin
	}{
		{"num.add", binary("a + b", t, func(a, b float64) (float64, bool) { return a + b, true }, sumUnit,
			func(c, a float64) (float64, bool) { return c - a, true }, func(c, a ns.Unit) (ns.Unit, bool) { return sumUnit(c, a) },
			func(c, b float64) (float64, bool) { return c - b, true }, func(c, b ns.Unit) (ns.Unit, bool) { return sumUnit(c, b) })},
		{"num.sub", binary("a - b", t, func(a, b float64) (float64, bool) { return a - b, true }, sumUnit,
			func(c, a float64) (float64, bool) { return a - c, true }, func(c, a ns.Unit) (ns.Unit, bool) { return sumUnit(c, a) },
			func(c, b float64) (float64, bool) { return c + b, true }, func(c, b ns.Unit) (ns.Unit, bool) { return sumUnit(c, b) })},
		{"num.mul", binary("a * b", t, func(a, b float64) (float64, bool) { return a * b, true }, mulUnit,
			func(c, a float64) (float64, bool) { return c / a, a != 0 }, divUnit,
			func(c, b float64) (float64, bool) { return c / b, b != 0 }, divUnit)},
		{"num.div", binary("a / b", t, func(a, b float64) (float64, bool) { return a / b, b != 0 }, divUnit,
			func(c, a float64) (float64, bool) { return a / c, c != 0 }, func(c, a ns.Unit) (ns.Unit, bool) { return divUnit(a, c) },
			func(c, b float64) (float64, bool) { return c * b, true }, mulUnit)},
		{"num.neg", negate(t)},
	} {
		if err := v.AddPredicate(p.path, p.b); err != nil {
			return err
		}
	}
	return nil
}

func sumUnit(a, b ns.Unit) (ns.Unit, bool) {
	switch {
	case a.IsZero():
		return b, true
	case b.IsZero(), a.Same(b):
		return a, true
	}
	return ns.Unit{}, false
}

func mulUnit(a, b ns.Unit) (ns.Unit, bool) { return a.Mul(b), true }
func divUnit(a, b ns.Unit) (ns.Unit, bool) { return a.Div(b), true }

type (
	numFn  func(x, y float64) (float64, bool)
	unitFn func(x, y ns.Unit) (ns.Unit, bool)
)

// binary is a function c = f(a, b) as a three-place relation: forward from a and b, and backward to a
// or b from c and the other (ba and bb, with their units), each mode reading what it has bound.
func binary(doc string, units func() *ns.Units, f numFn, fu unitFn, toB numFn, toBu unitFn, toA numFn, toAu unitFn) ns.Builtin {
	return ns.Builtin{
		Arity:  3,
		Modes:  [][]bool{{true, true, false}, {true, false, true}, {false, true, true}},
		Labels: []string{"a", "b", "c"},
		Types:  []ns.ArgType{{Type: ns.TypeNumber}, {Type: ns.TypeNumber}, {Type: ns.TypeNumber}},
		Doc:    "c = " + doc + ", run from any two of the three; units follow the vocabulary's table",
		Gen: func(_ context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			t := units()
			num := func(i int) (float64, ns.Unit, bool) {
				x := args[i].Value
				if !args[i].Bound || x.Num == nil {
					return 0, ns.Unit{}, false
				}
				u, err := t.ParseUnit(x.BaseUnit)
				if x.BaseUnit == "" {
					u, err = ns.Unit{}, nil
				}
				return *x.Num, u, err == nil
			}
			a, ua, okA := num(0)
			b, ub, okB := num(1)
			c, uc, okC := num(2)
			out := func(i int, x float64, u ns.Unit) error {
				f, unit, err := t.Write(x, u)
				if err != nil {
					return nil
				}
				vals := []ns.Value{args[0].Value, args[1].Value, args[2].Value}
				vals[i] = ns.NU(f, unit)
				return emit(vals, nil)
			}
			switch {
			case okA && okB:
				if x, ok := f(a, b); ok {
					if u, ok := fu(ua, ub); ok {
						return out(2, x, u)
					}
				}
				return nil
			case okA && okC:
				if x, ok := toB(c, a); ok {
					if u, ok := toBu(uc, ua); ok {
						return out(1, x, u)
					}
				}
				return nil
			case okB && okC:
				if x, ok := toA(c, b); ok {
					if u, ok := toAu(uc, ub); ok {
						return out(0, x, u)
					}
				}
				return nil
			}
			if args[0].Bound && args[1].Bound || args[0].Bound && args[2].Bound || args[1].Bound && args[2].Bound {
				return nil // a bound value that isn't a number: no result
			}
			return errors.New("query: arithmetic needs two of its three values bound (a variable must appear in a relation before it is computed with)")
		},
	}
}

func negate(units func() *ns.Units) ns.Builtin {
	return ns.Builtin{
		Arity:  2,
		Modes:  [][]bool{{true, false}, {false, true}},
		Labels: []string{"a", "b"},
		Types:  []ns.ArgType{{Type: ns.TypeNumber}, {Type: ns.TypeNumber}},
		Doc:    "b = -a, run from either; the unit carries over",
		Gen: func(_ context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			for i, j := range []int{1, 0} {
				if x := args[i].Value; args[i].Bound {
					if x.Num == nil {
						return nil
					}
					vals := []ns.Value{args[0].Value, args[1].Value}
					vals[j] = ns.NU(-*x.Num, x.BaseUnit)
					return emit(vals, nil)
				}
			}
			return errors.New("query: arithmetic needs a value bound to negate (a variable must appear in a relation before it is computed with)")
		},
	}
}

// Units registers the SI prefixes (T, G, M, k, m, u and µ, n, p), base units for current, potential,
// length, mass and time (A, V, m, g, s), and the electrical units over them: ohm (and Ω) = V/A, W =
// V*A, J = W*s, C = A*s, F = C/V, H = V*s/A, and Hz = 1/s. A host adds its own with
// Vocabulary.AddUnit, and a symbol nobody defines is a unit of its own.
func Units(v *ns.Vocabulary) error {
	for _, u := range [][2]string{
		{"T", "1e12"}, {"G", "1e9"}, {"M", "1e6"}, {"k", "1e3"}, {"m", "1e-3"}, {"u", "1e-6"}, {"µ", "1e-6"}, {"n", "1e-9"}, {"p", "1e-12"},
		{"A", ""}, {"V", ""}, {"m", ""}, {"g", ""}, {"s", ""},
		{"ohm", "V/A"}, {"Ω", "V/A"}, {"W", "V*A"}, {"J", "W*s"}, {"C", "A*s"}, {"F", "C/V"}, {"H", "V*s/A"}, {"Hz", "1/s"},
	} {
		if err := v.AddUnit(u[0], u[1]); err != nil {
			return err
		}
	}
	return nil
}
