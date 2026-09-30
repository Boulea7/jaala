package datalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// agniCatalog is agni's relation catalog as of agni#751's renames, each relation written as a rule
// head declaring its argument types. It is transcribed from the declarations behind agni's
// service/testdata/columnkinds.golden, so the snapshot below checks the port against agni's own
// answers rather than against itself.
var agniCatalog = []string{
	`board.layer(?net: net, ?layer)`,
	`board.track_width(?net: net, ?mm)`,
	`board.via_drill(?net: net, ?mm)`,
	`bus(?label, ?kind)`,
	`component.attr(?ref_des: component, ?key, ?value)`,
	`component.class(?ref_des: component, ?class)`,
	`component.device_class(?ref_des: component, ?class)`,
	`component.esd_rated(?ref_des: component)`,
	`component.mpn(?ref_des: component, ?mpn)`,
	`component.net(?ref_des: component, ?net: net)`,
	`component.net_count(?ref_des: component, ?count)`,
	`component.pin(?ref_des: component, ?pin: pin(?ref_des))`,
	`design.has_nc_channel(?present)`,
	`design.has_netclass(?present)`,
	`design.has_netclass_defs(?present)`,
	`design.types_power_out(?present)`,
	`entity(?name: ?kind, ?kind: {"component", "net", "bus"})`,
	`net.ac_coupled(?net: net)`,
	`net.attr(?net: net, ?key, ?value)`,
	`net.bias(?net: net, ?level)`,
	`net.bus_like(?net: net)`,
	`net.connector_signal(?net: net)`,
	`net.declared_track_width(?net: net, ?mm)`,
	`net.declared_via_drill(?net: net, ?mm)`,
	`net.external(?net: net)`,
	`net.feedback(?net: net)`,
	`net.ground(?net: net)`,
	`net.max_voltage(?net: net, ?volts)`,
	`net.netclass(?net: net, ?class)`,
	`net.nominal_voltage(?net: net, ?volts)`,
	`net.pin_count(?net: net, ?count)`,
	`net.rail(?net: net)`,
	`net.role(?net: net, ?role)`,
	`net.signal_level(?net: net, ?volts)`,
	`net.switching(?net: net)`,
	`netclass.clearance(?class, ?mm)`,
	`netclass.track_width(?class, ?mm)`,
	`netclass.via_diameter(?class, ?mm)`,
	`netclass.via_drill(?class, ?mm)`,
	`param.max(?mpn, ?symbol, ?max)`,
	`param.pin(?mpn, ?pin, ?name, ?function)`,
	`param.pin_range(?mpn, ?pin, ?symbol, ?kind, ?min, ?max)`,
	`param.pin_relation(?mpn, ?subject_pin, ?reference_pin, ?modality, ?min, ?max)`,
	`param.prov(?mpn, ?symbol, ?doc, ?page, ?section)`,
	`param.range(?mpn, ?symbol, ?kind, ?min, ?max)`,
	`param.typ(?mpn, ?symbol, ?typ)`,
	`param.unit(?mpn, ?symbol, ?unit)`,
	`part.audience(?mpn, ?who)`,
	`pin.name(?ref_des: component, ?pin: pin(?ref_des), ?name)`,
	`pin.net(?ref_des: component, ?pin: pin(?ref_des), ?net: net)`,
	`pin.role(?ref_des: component, ?pin: pin(?ref_des), ?role)`,
	`pin.type(?ref_des: component, ?pin: pin(?ref_des), ?etype)`,
	`reader.pin_net_conflict(?ref_des: component, ?pin: pin(?ref_des), ?net: net)`,
	`reader.ref_des_collision(?ref_des: component)`,
	`reader.unresolved_symbol(?ref_des: component, ?symref)`,
}

// agniRegistry registers agniCatalog, the standard predicates, and agni's two graph walks.
func agniRegistry(t *testing.T) *Registry {
	t.Helper()
	src := NewMemSource()
	for _, d := range agniCatalog {
		head, types, err := parseHead(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		labels := make([]string, len(head.Args))
		for i, a := range head.Args {
			labels[i] = string(a.Var)
		}
		for i := range types {
			for _, ref := range []*string{&types[i].KindFrom, &types[i].Owner} {
				if *ref != "" {
					*ref = labels[headIndex(head, Var(*ref))]
				}
			}
		}
		src.DeclareSchema(head.Relation, Schema{Arity: len(labels), Labels: labels, Types: types})
	}
	r := std(src)
	walk := func(src Source, args []Arg, emit func([]Value, []string) error) error { return nil }
	for path, b := range map[string]Builtin{
		"net.reaches": {Arity: 2, MaxArity: 3, Gen: walk, Labels: []string{"from", "net", "hops?"}, Types: []ArgType{{Kind: "net"}, {Kind: "net"}}},
		"net.route":   {Arity: 3, Gen: walk, Labels: []string{"from", "net", "path"}, Types: []ArgType{{Kind: "net"}, {Kind: "net"}}},
	} {
		if err := r.AddPredicate(path, b); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// The corpus is one generated query per member, `rel(?a0, ..., ?aN) => ?a0, ..., ?aN`, the same shape
// agni's snapshot generates, rendered in agni's format. It should differ from agni's golden only by the
// renamed paths; testdata/columnkinds.golden is agni's file with agni#751's renames applied and
// re-sorted. Regenerate with UPDATE_GOLDEN=1 only after reading why a line moved.
func TestColumnKindsMatchAgnisGolden(t *testing.T) {
	r := agniRegistry(t)
	var b strings.Builder
	var walk func(module string)
	walk = func(module string) {
		members, err := r.Members(module)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range members {
			if m.Kind == EntryModule {
				walk(m.Path)
				continue
			}
			vars := make([]string, len(m.Args))
			for i := range vars {
				vars[i] = fmt.Sprintf("?a%d", i)
			}
			joined := strings.Join(vars, ", ")
			ks, err := ColumnKinds(MustParse(fmt.Sprintf("%s(%s) => %s", m.Path, joined, joined)), r)
			if err != nil {
				t.Fatalf("%s: %v", m.Path, err)
			}
			for i, k := range ks {
				ref := "-"
				switch {
				case k.Owner.Var != "":
					ref = "?" + string(k.Owner.Var)
				case k.Owner.Const != nil:
					ref = strconv.Quote(k.Owner.Const.S)
				}
				fmt.Fprintf(&b, "%s\targ%d=%s\tkind=%q\tkindVar=%q\tref=%s\n", m.Path, i, m.Args[i].Name, k.Kind, k.KindFrom, ref)
			}
		}
	}
	walk("")
	got := sortedLines(b.String())
	path := filepath.Join("testdata", "columnkinds.golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		w, g := strings.Split(string(want), "\n"), strings.Split(got, "\n")
		for i := 0; i < len(w) || i < len(g); i++ {
			if i >= len(w) || i >= len(g) || w[i] != g[i] {
				t.Fatalf("column kinds differ from agni's at line %d\n want: %s\n got:  %s", i+1, at(w, i), at(g, i))
			}
		}
	}
}

// sortedLines orders the snapshot by path, keeping each member's arguments in order, as the golden is.
func sortedLines(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	sortStableByPath(lines)
	return strings.Join(lines, "\n") + "\n"
}

func sortStableByPath(lines []string) {
	key := func(l string) string { p, _, _ := strings.Cut(l, "\t"); return p }
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && key(lines[j]) < key(lines[j-1]); j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "(end)"
}
