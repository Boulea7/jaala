package datalog

import (
	"github.com/panyam/jaala/ns"
	"reflect"
	"strings"
	"testing"
	"time"
)

// circuit is a registry shaped like agni's, under the paths agni#751 moves them to: enough relations
// to carry every kind of declaration its column typing reads.
func circuit() *ns.Vocabulary {
	src := ns.NewMemSource().
		DeclareSchema("component.class", ns.Schema{Arity: 2, Labels: []string{"ref_des", "class"}, Types: []ns.ArgType{{Kind: "component"}, {Type: ns.TypeString}}}).
		DeclareSchema("component.net", ns.Schema{Arity: 2, Labels: []string{"ref_des", "net"}, Types: []ns.ArgType{{Kind: "component"}, {Kind: "net"}}, Doc: "a component is on a net"}).
		DeclareSchema("component.pin", ns.Schema{Arity: 2, Labels: []string{"ref_des", "pin"}, Types: []ns.ArgType{{Kind: "component"}, {Kind: "pin", Owner: "ref_des"}}}).
		DeclareSchema("component.mpn", ns.Schema{Arity: 2, Labels: []string{"ref_des", "mpn"}, Types: []ns.ArgType{{Kind: "component"}}}).
		DeclareSchema("net.ground", ns.Schema{Arity: 1, Labels: []string{"net"}, Types: []ns.ArgType{{Kind: "net"}}}).
		DeclareSchema("net.max_voltage", ns.Schema{Arity: 2, Labels: []string{"net", "volts"}, Types: []ns.ArgType{{Kind: "net"}, {Type: ns.TypeNumber, Unit: "V"}}}).
		DeclareSchema("entity", ns.Schema{Arity: 2, Labels: []string{"name", "kind"}, Types: []ns.ArgType{{KindFrom: "kind"}, {Domain: []string{"component", "net", "bus"}}}})
	src.Add("component.class", ns.Tuple{Vals: []ns.Value{ns.S("L1"), ns.S("ferrite")}}).
		Add("component.net", ns.Tuple{Vals: []ns.Value{ns.S("L1"), ns.S("VBUS")}}).
		Add("entity", ns.Tuple{Vals: []ns.Value{ns.S("VBUS"), ns.S("net")}})
	return std(src)
}

func kinds(t *testing.T, r *ns.Vocabulary, q string) []ColumnKind {
	t.Helper()
	got, err := ColumnKinds(mustParse(t, q), r)
	if err != nil {
		t.Fatalf("ColumnKinds(%q): %v", q, err)
	}
	return got
}

// agni's column-typing cases (service/query_test.go, agni issue 654), paths renamed per agni#751.
func TestColumnKindsFollowDerivedRelations(t *testing.T) {
	for _, c := range []struct{ name, query, want string }{
		{"a declared atom", `component.class(?p, "ferrite") => ?p`, "component"},
		{"one hop through a rule", `ferr(?p) :- component.class(?p, "ferrite"); ferr(?p) => ?p`, "component"},
		{"two hops", `a(?p) :- component.class(?p, "ferrite"); b(?p) :- a(?p); b(?p) => ?p`, "component"},
		{"a bucket through negation",
			`p(?c) :- component.class(?c, "capacitor"); cov(?c) :- p(?c), component.net(?c, ?n); ` +
				`nocov(?c) :- p(?c), not cov(?c); nocov(?c) => ?c`, "component"},
		// The control: without it a first-wins implementation passes every other case.
		{"rules that disagree stay untyped", `x(?v) :- component.class(?v, "ferrite"); x(?v) :- net.ground(?v); x(?v) => ?v`, ""},
		{"a per-row kind the head drops stays untyped", `e(?n) :- entity(?n, ?k); e(?n) => ?n`, ""},
		{"an aggregate is a number whatever it reduces", `ferr(?p) :- component.class(?p, "ferrite"); ferr(?p) => count(?p)`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := kinds(t, circuit(), c.query); len(got) != 1 || got[0].Kind != c.want || got[0].KindFrom != "" {
				t.Errorf("ColumnKinds = %+v, want kind %q", got, c.want)
			}
		})
	}
}

func TestColumnKindsSurvivesARecursiveRule(t *testing.T) {
	done := make(chan []ColumnKind, 1)
	go func() {
		got, _ := ColumnKinds(MustParse(`r(?a, ?b) :- component.net(?a, ?b); r(?a, ?b) :- r(?a, ?c), r(?c, ?b); r(?a, ?b) => ?a`), circuit())
		done <- got
	}()
	select {
	case got := <-done:
		if len(got) != 1 || got[0].Kind != "component" {
			t.Errorf("ColumnKinds = %+v, want component, typed by the base case", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ColumnKinds did not terminate on a recursive rule set")
	}
}

func TestAPerRowKindStaysPerRowThroughAHeadThatCarriesIt(t *testing.T) {
	if got := kinds(t, circuit(), `entity(?n, ?k) => ?n`); got[0].KindFrom != "k" || got[0].Kind != "" {
		t.Errorf("entity column = %+v, want its kind from ?k", got[0])
	}
	if got := kinds(t, circuit(), `e(?n, ?k) :- entity(?n, ?k); e(?name, ?what) => ?name`); got[0].KindFrom != "what" {
		t.Errorf("through a rule = %+v, want its kind from ?what", got[0])
	}
}

func TestAConstantKindTypesTheColumnOnlyWhenTheVocabularyHoldsIt(t *testing.T) {
	if got := kinds(t, circuit(), `entity(?n, "net") => ?n`); got[0].Kind != "net" {
		t.Errorf("entity(?n, \"net\") = %+v, want net", got[0])
	}
	if got := kinds(t, circuit(), `entity(?n, "pin") => ?n`); got[0] != (ColumnKind{}) {
		t.Errorf("entity(?n, \"pin\") = %+v, want untyped", got[0])
	}
}

func TestAPinIsLocatedThroughItsOwner(t *testing.T) {
	if got := kinds(t, circuit(), `component.pin(?r, ?p) => ?p`); got[0].Kind != "pin" || got[0].Owner.Var != "r" {
		t.Errorf("pin column = %+v, want pin owned by ?r", got[0])
	}
	if got := kinds(t, circuit(), `pp(?c, ?q) :- component.pin(?c, ?q); pp(?r, ?p) => ?p`); got[0].Kind != "pin" || got[0].Owner.Var != "r" {
		t.Errorf("through a rule = %+v, want pin owned by ?r", got[0])
	}
	if got := kinds(t, circuit(), `pp(?q) :- component.pin(?c, ?q); pp(?p) => ?p`); got[0] != (ColumnKind{}) {
		t.Errorf("owner dropped = %+v, want untyped: a pin nothing locates", got[0])
	}
}

func TestAScalarColumnKeepsItsTypeAndUnit(t *testing.T) {
	got := kinds(t, circuit(), `net.max_voltage(?n, ?v) => ?n, ?v`)
	if got[0].Kind != "net" || got[1].Type != ns.TypeNumber || got[1].Unit != "V" {
		t.Errorf("ColumnKinds = %+v, want net then number[V]", got)
	}
}

func TestParseHeadDeclarations(t *testing.T) {
	q := mustParse(t, `x(?n: net, ?v: number[V], ?k, ?e: ?k, ?p: pin(?n), ?r: {"source", "sink"}, ?s: string) :- `+
		`component.net(?n, ?n), net.max_voltage(?n, ?v), entity(?e, ?k), component.pin(?n, ?p), component.class(?r, ?s), component.mpn(?s, ?s); x(?n, ?v, ?k, ?e, ?p, ?r, ?s)`)
	var got []string
	for _, ty := range q.Rules[0].HeadTypes {
		got = append(got, ty.String())
	}
	want := []string{"net", "number[V]", "", "?k", "pin(?n)", `{"source", "sink"}`, "string"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("declarations = %q, want %q", got, want)
	}
	for text, frag := range map[string]string{
		`x(?n: ?n) :- node(?n); x(?n)`:        "must name another variable of its head",
		`x(?n: pin(?r)) :- node(?n); x(?n)`:   "must name another variable of its head",
		`x("a": net) :- node(?n); x(?n)`:      "only a ?variable can declare a type",
		`x(?n: {source}) :- node(?n); x(?n)`:  "must be a \"string\"",
		`x(?n: net thing) :- node(?n); x(?n)`: "bad type declaration",
	} {
		if _, err := Parse(text); err == nil || !strings.Contains(err.Error(), frag) {
			t.Errorf("%s: err = %v, want %q", text, err, frag)
		}
	}
}

// power is a module whose members show each way a signature is arrived at.
const power = `
# Nets that carry a test point.
has_test_point(?n) :- component.net(?tp, ?n), component.class(?tp, "test_point");

# A net's role: declared closed, both values from constant heads.
role(?n, "source") :- net.ground(?n);
role(?n, "sink") :- has_test_point(?n);

# Declares a kind its body cannot contradict, since mpn is untyped.
part(?m: part) :- component.mpn(_, ?m);

_helper(?x) :- net.ground(?x);
grounded(?x) :- _helper(?x);
`

func TestSignaturesAreDeclaredOrInferredAndMarked(t *testing.T) {
	r := circuit()
	if err := r.AddModule("net", LanguageName, power, ""); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"net.has_test_point": "net.has_test_point(n: net)",
		"net.role":           `net.role(n: net, arg1: {"sink", "source"})`,
		"net.part":           "net.part(m: part)",
		"net.grounded":       "net.grounded(x: net)",
	} {
		e, err := r.Lookup(path)
		if err != nil {
			t.Fatal(err)
		}
		if e.Signature() != want {
			t.Errorf("%s = %s, want %s", path, e.Signature(), want)
		}
	}
	e, _ := r.Lookup("net.part")
	if e.Args[0].Inferred {
		t.Error("net.part's declared argument is marked inferred")
	}
	e, _ = r.Lookup("net.has_test_point")
	if !e.Args[0].Inferred {
		t.Error("net.has_test_point's argument is not marked inferred")
	}
}

func TestADeclarationItsRulesContradictIsRefused(t *testing.T) {
	for text, frag := range map[string]string{
		`x(?n: component) :- net.ground(?n);`:                               `net.x declares ?n: component, but its rules make it net`,
		`x(?v: number[A]) :- net.max_voltage(_, ?v);`:                       `declares ?v: number[A], but its rules make it number[V]`,
		`x(?r: {"a", "b"}) :- entity(_, ?r);`:                               `able to hold "bus"`,
		`x(?n: net) :- net.ground(?n); x(?n: component) :- net.ground(?n);`: `declares its "n" argument as both "net" and "component"`,
	} {
		r := circuit()
		if err := r.AddModule("net", LanguageName, text, ""); err != nil {
			t.Fatal(err)
		}
		if err := r.Check(); err == nil || !strings.Contains(err.Error(), frag) {
			t.Errorf("%s\n err = %v\n want %q", text, err, frag)
		}
	}
}

func TestADerivedVocabularyIsEnforcedInQueries(t *testing.T) {
	r := circuit()
	if err := r.AddModule("net", LanguageName, power, ""); err != nil {
		t.Fatal(err)
	}
	_, err := Naive{}.Eval(mustParse(t, `net.role(?n, "sorce")`), baseFor(r))
	if err == nil || !strings.Contains(err.Error(), `net.role's "arg1" argument cannot be "sorce", did you mean "source"?`) {
		t.Errorf("err = %v, want the vocabulary enforced with a suggestion", err)
	}
}

func TestLookupDrillsFromModuleToMemberToDefinition(t *testing.T) {
	r := circuit()
	if err := r.AddModule("net", LanguageName, power, ""); err != nil {
		t.Fatal(err)
	}
	root, err := r.Lookup("")
	if err != nil || root.Kind != ns.EntryModule || !reflect.DeepEqual(root.Members, []string{"absent", "component", "entity", "net", "str"}) {
		t.Errorf("root = %+v, %v", root, err)
	}
	members, err := r.Members("net")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, m := range members {
		listed = append(listed, string(m.Kind)+" "+m.Signature())
	}
	want := []string{
		"base net.ground(net: net)",
		"derived net.grounded(x: net)",
		"derived net.has_test_point(n: net)",
		"base net.max_voltage(net: net, volts: number[V])",
		"derived net.part(m: part)",
		`derived net.role(n: net, arg1: {"sink", "source"})`,
	}
	if !reflect.DeepEqual(listed, want) {
		t.Errorf("Members(net) =\n %s\nwant\n %s", strings.Join(listed, "\n "), strings.Join(want, "\n "))
	}
	e, _ := r.Lookup("net.role")
	wantDef := []string{`role(?n, "source") :- net.ground(?n)`, `role(?n, "sink") :- has_test_point(?n)`}
	if e.Doc != "A net's role: declared closed, both values from constant heads." || e.Module != "net" || !reflect.DeepEqual(e.Definition, wantDef) {
		t.Errorf("net.role = %+v, want its doc and its two rules as written", e)
	}
	if e, _ := r.Lookup("component.net"); e.Kind != ns.EntryBase || e.Doc != "a component is on a net" {
		t.Errorf("component.net = %+v", e)
	}
	if e, _ := r.Lookup("str.contains"); e.Kind != ns.EntryPredicate || e.Signature() != "str.contains(string: string, substring: string)" {
		t.Errorf("str.contains = %+v", e)
	}
	if _, err := r.Lookup("net._helper"); err == nil || !strings.Contains(err.Error(), `unknown relation "net._helper"`) {
		t.Errorf("private: err = %v, want it unknown", err)
	}
	if _, err := r.Lookup("net.has_tst_point"); err == nil || !strings.Contains(err.Error(), `did you mean "net.has_test_point"?`) {
		t.Errorf("typo: err = %v, want a suggestion", err)
	}
	if _, err := r.Members("net.role"); err == nil || !strings.Contains(err.Error(), "is a derived, not a module") {
		t.Errorf("Members of a member: err = %v", err)
	}
}
