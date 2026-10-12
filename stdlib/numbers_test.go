package stdlib

import (
	"context"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

func TestNumberPartsSplitAValueFromItsUnit(t *testing.T) {
	v := ns.MustVocabulary(nil)
	if err := Register(v); err != nil {
		t.Fatal(err)
	}
	emitted := func(path string, x ns.Value) []ns.Value {
		t.Helper()
		b, ok := v.Predicate(path)
		if !ok || !b.IsGenerator() || !b.Satisfied([]bool{true, false}) || b.Satisfied([]bool{false, true}) {
			t.Fatalf("%s = %+v, %v, want a generator needing its number bound", path, b, ok)
		}
		var got []ns.Value
		err := b.Gen(context.Background(), nil, []ns.Arg{{Value: x, Bound: true}, {}}, func(vals []ns.Value, _ []string) error {
			got = append(got, vals[1])
			return nil
		})
		if err != nil {
			t.Fatalf("%s(%v): %v", path, x, err)
		}
		return got
	}
	if got := emitted("num.value", ns.NU(5, "V")); len(got) != 1 || got[0].Num == nil || *got[0].Num != 5 || got[0].BaseUnit != "" {
		t.Errorf("num.value(5 V) = %+v, want 5 with no unit", got)
	}
	if got := emitted("num.unit", ns.NU(5, "V")); len(got) != 1 || got[0].S != "V" || got[0].Num != nil {
		t.Errorf("num.unit(5 V) = %+v, want V", got)
	}
	if got := emitted("num.unit", ns.N(5)); len(got) != 1 || !got[0].Absent {
		t.Errorf("num.unit(5) = %+v, want absent, not \"\"", got)
	}
	for _, path := range []string{"num.value", "num.unit"} {
		for _, x := range []ns.Value{ns.S("five"), ns.Absent()} {
			if got := emitted(path, x); len(got) != 0 {
				t.Errorf("%s(%+v) = %+v, want nothing: it isn't a number", path, x, got)
			}
		}
		b, _ := v.Predicate(path)
		if err := b.Gen(context.Background(), nil, []ns.Arg{{}, {}}, func([]ns.Value, []string) error {
			t.Errorf("%s emitted with its number unbound", path)
			return nil
		}); err == nil || !strings.HasPrefix(err.Error(), "query: ") {
			t.Errorf("%s with its number unbound: err = %v, want a query: error", path, err)
		}
	}
}
