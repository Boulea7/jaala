package datalog

import (
	"reflect"
	"testing"

	"github.com/panyam/jaala/ns"
)

// typedNets has pin counts whose text order and number order disagree (10 sorts before 2 as text), a
// part whose reference looks numeric, a pin stored as a number in an entity position, and an untyped
// relation holding the same numbers as the counts.
func typedNets() *ns.MemSource {
	src := ns.NewMemSource().
		DeclareSchema("net.pin_count", ns.Schema{Arity: 2, Labels: []string{"net", "count"}, Types: []ns.ArgType{{Kind: "net"}, {Type: ns.TypeNumber}}}).
		DeclareSchema("component.net", ns.Schema{Arity: 2, Labels: []string{"ref_des", "net"}, Types: []ns.ArgType{{Kind: "component"}, {Kind: "net"}}}).
		DeclareSchema("component.pin", ns.Schema{Arity: 2, Labels: []string{"ref_des", "pin"}, Types: []ns.ArgType{{Kind: "component"}, {Kind: "pin", Owner: "ref_des"}}}).
		DeclareSchema("raw", ns.Schema{Arity: 1})
	src.Add("net.pin_count", ns.Tuple{Vals: []ns.Value{ns.S("VBUS"), ns.N(10)}}).
		Add("net.pin_count", ns.Tuple{Vals: []ns.Value{ns.S("GND"), ns.N(2)}}).
		Add("component.net", ns.Tuple{Vals: []ns.Value{ns.S("3"), ns.S("VBUS")}}).
		Add("component.net", ns.Tuple{Vals: []ns.Value{ns.S("R1"), ns.S("GND")}}).
		Add("component.pin", ns.Tuple{Vals: []ns.Value{ns.S("U1"), ns.N(3)}}).
		Add("raw", ns.Tuple{Vals: []ns.Value{ns.N(10)}}).
		Add("raw", ns.Tuple{Vals: []ns.Value{ns.N(2)}})
	return src
}

// answersAs checks that query, with opts, answers exactly as want does, and that want answers
// something, so two empty answers cannot agree.
func answersAs(t *testing.T, query, want string, opts ...Option) {
	t.Helper()
	b := baseFor(std(typedNets()))
	got, err := both(mustParse(t, query), b, opts...)
	if err != nil {
		t.Fatalf("Eval(%q): %v", query, err)
	}
	exp, err := both(mustParse(t, want), b)
	if err != nil {
		t.Fatalf("Eval(%q): %v", want, err)
	}
	if len(exp) == 0 {
		t.Fatalf("control: %q answers nothing, so it cannot tell a coerced constant from a missed one", want)
	}
	if !reflect.DeepEqual(binds(got), binds(exp)) {
		t.Errorf("%q answers %v, want %v (as %q)", query, binds(got), binds(exp), want)
	}
}

func TestTextInANumberPositionIsANumber(t *testing.T) {
	answersAs(t, `net.pin_count(?n, ?c), ?c >= "3" => ?n`, `net.pin_count(?n, ?c), ?c >= 3 => ?n`)
	answersAs(t, `net.pin_count(?n, ?c), "3" <= ?c => ?n`, `net.pin_count(?n, ?c), ?c >= 3 => ?n`)
	answersAs(t, `net.pin_count(?n, "10.0") => ?n`, `net.pin_count(?n, 10) => ?n`)
	answersAs(t, `net.pin_count(?n, ?c), ?c >= ?min => ?n`, `net.pin_count(?n, ?c), ?c >= 3 => ?n`,
		Bind(map[Var]ns.Value{"min": ns.S("3")}))
}

func TestANumberInAnEntityPositionMatchesByItsText(t *testing.T) {
	answersAs(t, `component.net(3, ?n) => ?n`, `component.net("3", ?n) => ?n`)
	answersAs(t, `component.net(?r, ?n) => ?n`, `component.net("3", ?n) => ?n`, Bind(map[Var]ns.Value{"r": ns.N(3)}))
	// Pin 3.0 is not pin 3, even when the Source stored the pin as a number.
	if rows := eval(t, typedNets(), `component.pin(?r, 3.0) => ?r`); len(rows) != 0 {
		t.Errorf("pin 3.0 matched pin 3: %v", binds(rows))
	}
	if rows := eval(t, typedNets(), `component.pin(?r, 3) => ?r`); col(rows, "r") != "U1" {
		t.Errorf("control: pin 3 answers %q, want U1", col(rows, "r"))
	}
}

func TestTextANumberPositionCannotReadIsRefused(t *testing.T) {
	src := typedNets()
	for _, c := range []struct {
		query string
		bind  map[Var]ns.Value
		want  string
	}{
		{`net.pin_count(?n, ?c), ?c >= "abc" => ?n`, nil,
			`query: ?c cannot be compared with "abc": it is a number (net.pin_count's "count" argument)`},
		{`net.pin_count(?n, "abc") => ?n`, nil, `query: net.pin_count's "count" argument cannot be "abc" (it holds a number)`},
		{`net.pin_count(?n, ?c), not net.pin_count(?n, "abc") => ?n`, nil,
			`query: net.pin_count's "count" argument cannot be "abc" (it holds a number)`},
		{`net.pin_count(?n, ?c), ?c >= ?min => ?n`, map[Var]ns.Value{"min": ns.S("abc")},
			`query: ?c cannot be compared with "abc": it is a number (net.pin_count's "count" argument)`},
		{`net.pin_count(?n, ?c) => ?n`, map[Var]ns.Value{"c": ns.S("abc")},
			`query: net.pin_count's "count" argument cannot be "abc" (it holds a number)`},
	} {
		_, err := both(mustParse(t, c.query), baseFor(std(src)), Bind(c.bind))
		if err == nil || err.Error() != c.want {
			t.Errorf("Eval(%q, %v) = %v, want %q", c.query, c.bind, err, c.want)
		}
		if c.bind == nil {
			if err := Validate(mustParse(t, c.query), std(src)); err == nil || err.Error() != c.want {
				t.Errorf("Validate(%q) = %v, want %q", c.query, err, c.want)
			}
		}
	}
}

// A rule body's constants are checked too, and a derived relation's number type is inferred from the
// relation it reads. Each relation has two rules, so inlining cannot fold it into the goal.
func TestConstantsInRulesAndDerivedRelationsAreTyped(t *testing.T) {
	rules := `big(?n) :- net.pin_count(?n, ?c), ?c >= "3"; big(?n) :- net.pin_count(?n, ?c), ?c > 100; ` +
		`cnt(?n, ?c) :- net.pin_count(?n, ?c); cnt(?n, ?c) :- net.pin_count(?n, ?c), ?c > 100; `
	answersAs(t, rules+`big(?n) => ?n`, `net.pin_count(?n, ?c), ?c >= 3 => ?n`)
	answersAs(t, rules+`cnt(?n, "10.0") => ?n`, `net.pin_count(?n, 10) => ?n`)
	err := evalErr(typedNets(), rules+`cnt(?n, "abc") => ?n`)
	if want := `query: cnt's "c" argument cannot be "abc" (it holds a number)`; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// Nothing types raw, so its constants keep the type they were written with: text and a number still
// have no order, and text it could never hold is not refused.
func TestAnUntypedPositionKeepsItsConstant(t *testing.T) {
	src := typedNets()
	if rows := eval(t, src, `raw(?x), ?x >= "3" => ?x`); len(rows) != 0 {
		t.Errorf("an untyped ?x was coerced: %v", binds(rows))
	}
	if rows := eval(t, src, `raw(?x), ?x >= 3 => ?x`); col(rows, "x") != "10" {
		t.Errorf("control: ?x >= 3 answers %q, want 10", col(rows, "x"))
	}
	if err := evalErr(src, `raw("abc")`); err != nil {
		t.Errorf("an untyped constant was refused: %v", err)
	}
}
