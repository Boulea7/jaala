package datalog

import (
	"strings"
	"testing"
)

// tree is a source whose relations sit at the root, one module deep and two deep.
func tree() *MemSource {
	src := NewMemSource().
		Declare("edge", "from", "to").
		Declare("net.pin_count", "net", "n").
		Declare("bus.pin_cont", "bus", "n").
		Declare("acme.power.rail_budget", "rail", "watts")
	src.Add("edge", Tuple{Vals: []Value{S("a"), S("b")}})
	src.Add("net.pin_count", Tuple{Vals: []Value{S("VBUS"), N(4)}})
	src.Add("acme.power.rail_budget", Tuple{Vals: []Value{S("VBUS"), N(10)}})
	return src
}

func TestPathsResolveAtEveryDepth(t *testing.T) {
	rows := eval(t, tree(), `edge(?a, ?b), net.pin_count(?n, ?c), acme.power.rail_budget(?n, ?w) => ?a, ?n, ?w`)
	if len(rows) != 1 || rows[0].Bind["a"].S != "a" || rows[0].Bind["w"].S != "10" {
		t.Errorf("rows = %v, want a root, a one-deep and a two-deep relation joined", rows)
	}
}

func TestASegmentCannotBeBothModuleAndMember(t *testing.T) {
	_, err := NewRegistry(NewMemSource().Declare("pin", "ref", "pin").Declare("pin.net", "pin", "net"))
	if err == nil || !strings.Contains(err.Error(), `needs "pin" to be a module`) {
		t.Errorf("member then module: err = %v, want pin refused as a module", err)
	}
	r := MustRegistry(tree())
	err = r.AddPredicate("acme.power", Filter(1, func([]Value) (bool, error) { return true, nil }))
	if err == nil || !strings.Contains(err.Error(), "already a module holding rail_budget") {
		t.Errorf("module then member: err = %v, want acme.power refused as a member", err)
	}
}

func TestOnePathHasOneDefiner(t *testing.T) {
	r := MustRegistry(tree())
	err := r.AddPredicate("net.pin_count", Filter(2, func([]Value) (bool, error) { return true, nil }))
	if err == nil || !strings.Contains(err.Error(), `"net.pin_count" is defined twice, as a base relation and as a predicate`) {
		t.Errorf("err = %v, want both definitions named", err)
	}
}

func TestAPathMustBeSpellable(t *testing.T) {
	r := MustRegistry(nil)
	for _, p := range []string{"", "a..b", ".a", "a b"} {
		if err := r.AddPredicate(p, Filter(1, func([]Value) (bool, error) { return true, nil })); err == nil {
			t.Errorf("AddPredicate(%q) was accepted", p)
		}
	}
}

func TestStringFiltersLiveUnderStrAndAbsentAtTheRoot(t *testing.T) {
	src := NewMemSource().Declare("row", "name", "min")
	src.Add("row", Tuple{Vals: []Value{S("VBUS"), Absent()}}).Add("row", Tuple{Vals: []Value{S("GND"), N(0)}})
	for q, want := range map[string]string{
		`row(?n, _), str.contains(?n, "BU") => ?n`: "VBUS",
		`row(?n, _), str.prefix(?n, "G") => ?n`:    "GND",
		`row(?n, _), str.suffix(?n, "D") => ?n`:    "GND",
		`row(?n, _), str.glob(?n, "V*") => ?n`:     "VBUS",
		`row(?n, _), str.match(?n, "^G") => ?n`:    "GND",
		`row(?n, ?m), absent(?m) => ?n`:            "VBUS",
	} {
		if got := col(eval(t, src, q), "n"); got != want {
			t.Errorf("%s = %s, want %s", q, got, want)
		}
	}
	err := evalErr(src, `row(?n, _), contains(?n, "BU")`)
	if err == nil || !strings.Contains(err.Error(), `unknown relation "contains"; did you mean "str.contains"?`) {
		t.Errorf("bare contains: err = %v, want it unknown with str.contains suggested", err)
	}
}

// A typo in a member is repaired inside its module, even when another module holds a closer spelling.
func TestUnknownMemberSuggestsFromItsOwnModule(t *testing.T) {
	err := evalErr(tree(), `net.pin_cont(?n, ?c)`)
	if err == nil || !strings.Contains(err.Error(), `unknown relation "net.pin_cont"; did you mean "net.pin_count"?`) {
		t.Errorf("err = %v, want net.pin_count and not bus.pin_cont", err)
	}
}

func TestAMissingModuleIsNamedAsOne(t *testing.T) {
	err := evalErr(tree(), `nett.pin_count(?n, ?c)`)
	if err == nil || !strings.Contains(err.Error(), `unknown module "nett" in "nett.pin_count"; did you mean "net.pin_count"?`) {
		t.Errorf("err = %v, want the module named and the repaired path suggested", err)
	}
	err = evalErr(tree(), `edge(?a, ?c), acme.powr.x(?a)`)
	if err == nil || !strings.Contains(err.Error(), `unknown module "acme.powr"`) || !strings.Contains(err.Error(), `did you mean "acme.power"?`) {
		t.Errorf("err = %v, want the nested module named and its nearest suggested", err)
	}
}

func TestAModuleIsNotARelation(t *testing.T) {
	err := evalErr(tree(), `edge(?a, ?b), str(?a)`)
	if err == nil || !strings.Contains(err.Error(), `"str" is a module, not a relation; it holds contains, glob, match, prefix, suffix`) {
		t.Errorf("err = %v, want str named as a module with its members", err)
	}
}

func TestDidYouMeanUsesTheTree(t *testing.T) {
	if got := DidYouMean(std(tree()), "net.pin_cont"); got != `; did you mean "net.pin_count"?` {
		t.Errorf("DidYouMean = %q", got)
	}
}
