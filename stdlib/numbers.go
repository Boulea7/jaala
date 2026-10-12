package stdlib

import (
	"context"
	"errors"

	"github.com/panyam/jaala/ns"
)

// Numbers registers the number predicates under num: num.value, which binds a number without its
// unit, and num.unit, which binds the unit. They are how a query says what it means when the engine
// won't guess: a min, max or sum over numbers in different units is absent (jaala#80), so a query
// that wants the raw numbers strips the units with num.value, and one that wants a result per unit
// groups by num.unit.
func Numbers(v *ns.Vocabulary) error {
	for _, p := range []struct {
		path string
		b    ns.Builtin
	}{
		{"num.value", numPart("value", ns.ArgType{Type: ns.TypeNumber},
			"binds the number without its unit, so 5 V and 5 A both bind 5; the number must be bound",
			func(x ns.Value) ns.Value { return ns.N(*x.Num) })},
		{"num.unit", numPart("unit", ns.ArgType{Type: ns.TypeString},
			"binds the number's unit (\"V\", \"A\"), or absent for a number without one; the number must be bound",
			func(x ns.Value) ns.Value {
				if x.BaseUnit == "" {
					return ns.Absent()
				}
				return ns.S(x.BaseUnit)
			})},
	} {
		if err := v.AddPredicate(p.path, p.b); err != nil {
			return err
		}
	}
	return nil
}

// numPart is a two-argument generator binding one part of a bound number. A value that isn't a number
// (text, absent) has no parts, so it emits nothing. With the second argument bound too the engine
// unifies the emitted part against it, so it tests.
func numPart(label string, t ns.ArgType, doc string, part func(ns.Value) ns.Value) ns.Builtin {
	path := "num." + label
	return ns.Builtin{
		Arity:  2,
		Modes:  [][]bool{{true, false}},
		Labels: []string{"number", label},
		Types:  []ns.ArgType{{Type: ns.TypeNumber}, t},
		Doc:    doc,
		Gen: func(_ context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			if !args[0].Bound {
				return errors.New("query: " + path + " needs its number bound (a variable must appear in a relation before " + path + " reads it)")
			}
			x := args[0].Value
			if x.Num == nil {
				return nil
			}
			return emit([]ns.Value{x, part(x)}, nil)
		},
	}
}
