package datalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

// params is a datasheet's parameters, one relation for every unit. U1's are in three units; U2's
// are all volts; U3 has a volt and a bare number, which takes the volt; U4 has 5 V and 5 A, one
// number in two units, so dedup on the number alone would hide the mix.
func params() *ns.MemSource {
	src := ns.NewMemSource().DeclareSchema("param", ns.Schema{Arity: 3, Labels: []string{"part", "sym", "value"}, Types: []ns.ArgType{{}, {}, {Type: ns.TypeNumber}}})
	for _, p := range []struct {
		part, sym string
		v         ns.Value
	}{
		{"U1", "vmax", ns.NU(5, "V")}, {"U1", "imax", ns.NU(0.5, "A")}, {"U1", "rpull", ns.NU(10000, "ohm")},
		{"U2", "vmax", ns.NU(3, "V")}, {"U2", "vmin", ns.NU(2, "V")},
		{"U3", "vref", ns.NU(1.25, "V")}, {"U3", "gain", ns.N(2)},
		{"U4", "vin", ns.NU(5, "V")}, {"U4", "iin", ns.NU(5, "A")},
	} {
		src.Add("param", ns.Tuple{Vals: []ns.Value{ns.S(p.part), ns.S(p.sym), p.v}})
	}
	return src
}

// withUnits writes each row's columns as text, a number with its unit in brackets, absent as - with
// the row's Why for it, if any, in parentheses.
func withUnits(rows []Row, cols ...string) []string {
	var out []string
	for _, r := range rows {
		var vals []string
		for _, c := range cols {
			v := r.Bind[Var(c)]
			switch {
			case v.Absent && r.Why[Var(c)] != "":
				vals = append(vals, "-("+r.Why[Var(c)]+")")
			case v.Absent:
				vals = append(vals, "-")
			case v.BaseUnit != "":
				vals = append(vals, v.S+"["+v.BaseUnit+"]")
			default:
				vals = append(vals, v.S)
			}
		}
		out = append(out, strings.Join(vals, " "))
	}
	return out
}

func TestAnAggregateOverMixedUnitsIsAbsent(t *testing.T) {
	rows := eval(t, params(), `param(?p, _, ?v) => ?p, max(?v), min(?v), sum(?v), count(?v)`)
	got := withUnits(rows, "p", "max(v)", "min(v)", "sum(v)", "count(v)")
	want := []string{
		"U1 -(mixed units: A, V, ohm) -(mixed units: A, V, ohm) -(mixed units: A, V, ohm) 3",
		"U2 3[V] 2[V] 5[V] 2", // control: one unit reduces, and keeps it
		"U3 2[V] 1.25[V] 3.25[V] 2",
		"U4 -(mixed units: A, V) -(mixed units: A, V) -(mixed units: A, V) 2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("per part = %v, want %v", got, want)
	}
	rows = eval(t, params(), `param(?p, _, ?v) => ?p, sum(distinct ?v)`)
	if got, want := withUnits(rows, "p", "sum(distinct v)"), []string{"U1 -(mixed units: A, V, ohm)", "U2 5[V]", "U3 3.25[V]", "U4 -(mixed units: A, V)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sum(distinct) = %v, want %v: U4's 5 V and 5 A are one number in two units", got, want)
	}
}

func TestAnAggregateInARuleOverMixedUnitsIsAbsent(t *testing.T) {
	for _, c := range []struct {
		name, query string
		want        []string
	}{
		{"head", `peak(?p, max(?v)) :- param(?p, _, ?v); peak(?p, ?m) => ?p, ?m`, []string{"U1 -", "U2 3[V]", "U3 2[V]", "U4 -"}},
		{"body", `param(?p, "vmax", _), ?m = max(?v) : { param(?p, _, ?v) } => ?p, ?m`, []string{"U1 -", "U2 3[V]"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := withUnits(eval(t, params(), c.query), "p", "m"); !reflect.DeepEqual(got, c.want) {
				t.Errorf("peaks = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNumberPartsSayWhatAMixedAggregateMeans(t *testing.T) {
	rows := eval(t, params(), `param("U1", _, ?v), num.value(?v, ?n) => max(?n)`)
	if got := withUnits(rows, "max(n)"); !reflect.DeepEqual(got, []string{"10000"}) {
		t.Errorf("U1's max without units = %v, want 10000", got)
	}
	rows = eval(t, params(), `param(?p, _, ?v), num.unit(?v, ?u), ?p != "U2" => ?p, ?u, max(?v)`)
	want := []string{"U1 A 0.5[A]", "U1 V 5[V]", "U1 ohm 10000[ohm]", "U3 - 2", "U3 V 1.25[V]", "U4 A 5[A]", "U4 V 5[V]"}
	if got := withUnits(rows, "p", "u", "max(v)"); !reflect.DeepEqual(got, want) {
		t.Errorf("max per unit = %v, want %v", got, want)
	}
}

// An aggregate a rule stores keeps only its value, so a goal reading it sees absent with no reason.
func TestWhyIsSetOnAGoalsOwnAggregates(t *testing.T) {
	for _, c := range []struct {
		query string
		cols  []string
		want  []string
	}{
		{`param(?p, ?s, _), ?p = "U2" => ?p, min(?s), sum(?s)`, []string{"min(s)", "sum(s)"}, []string{"-(no numbers) 0"}},
		{`param(?p, _, ?v), ?p = "U9" => max(?v), count(?v)`, []string{"max(v)", "count(v)"}, []string{"-(no rows) 0"}},
		{`peak(?p, max(?v)) :- param(?p, _, ?v); peak("U1", ?m) => ?m`, []string{"m"}, []string{"-"}},
	} {
		if got := withUnits(eval(t, params(), c.query), c.cols...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %v, want %v", c.query, got, c.want)
		}
	}
	rows := eval(t, params(), `param(?p, _, ?v) => ?p having max(?v) != 99`)
	if got := withUnits(rows, "p"); !reflect.DeepEqual(got, []string{"U1", "U2", "U3", "U4"}) {
		t.Fatalf("having = %v, want every part: absent isn't 99", got)
	}
	for _, r := range rows {
		if len(r.Why) != 0 {
			t.Errorf("%v carries Why %v for a column only having read", r.Bind, r.Why)
		}
	}
	rows = eval(t, params(), `param(?p, _, ?v), ?p = "U2" => ?p, max(?v)`)
	if len(rows) != 1 || rows[0].Why != nil {
		t.Errorf("a row with no absent column has Why %v, want nil", rows)
	}
}
