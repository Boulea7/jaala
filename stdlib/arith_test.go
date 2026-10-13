package stdlib

import (
	"context"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

func TestArithmeticRunsInEveryDirection(t *testing.T) {
	v := ns.MustVocabulary(nil)
	if err := Register(v); err != nil {
		t.Fatal(err)
	}
	run := func(path string, args ...ns.Value) string {
		t.Helper()
		b, _ := v.Predicate(path)
		in := make([]ns.Arg, len(args))
		for i, a := range args {
			in[i] = ns.Arg{Value: a, Bound: a != (ns.Value{})}
		}
		var out []string
		err := b.Gen(context.Background(), nil, in, func(vals []ns.Value, _ []string) error {
			for _, x := range vals {
				out = append(out, x.S+x.BaseUnit)
			}
			return nil
		})
		if err != nil {
			return "error: " + err.Error()
		}
		return strings.Join(out, " ")
	}
	free := ns.Value{}
	for _, c := range []struct{ got, want string }{
		{run("num.mul", ns.NU(0.002, "A"), ns.NU(1000, "ohm"), free), "0.002A 1000ohm 2V"},
		{run("num.mul", ns.NU(0.002, "A"), free, ns.NU(5, "V")), "0.002A 2500ohm 5V"},
		{run("num.mul", free, ns.NU(1000, "ohm"), ns.NU(5, "V")), "0.005A 1000ohm 5V"},
		{run("num.mul", ns.N(0), free, ns.N(5)), ""},
		{run("num.div", ns.NU(5, "V"), ns.NU(1000, "ohm"), free), "5V 1000ohm 0.005A"},
		{run("num.div", ns.N(1), ns.N(0), free), ""},
		{run("num.add", ns.NU(1, "V"), ns.N(2), free), "1V 2 3V"},
		{run("num.add", ns.N(2), ns.NU(1, "V"), free), "2 1V 3V"}, // a bare number takes the other side's unit, on either side
		{run("num.add", ns.NU(1, "V"), ns.NU(2, "A"), free), ""},
		{run("num.sub", ns.NU(5, "V"), free, ns.NU(2, "V")), "5V 3V 2V"},
		{run("num.neg", ns.NU(3, "A"), free), "3A -3A"},
		{run("num.neg", free, ns.N(3)), "-3 3"},
		{run("num.add", ns.S("five"), ns.N(1), free), ""},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if got := run("num.mul", ns.N(2), free, free); !strings.HasPrefix(got, "error: query: ") {
		t.Errorf("num.mul with one value bound = %q, want a query: error", got)
	}
}
