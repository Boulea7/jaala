package ns

import (
	"reflect"
	"strings"
	"testing"
)

type fixedVersion struct {
	*MemSource
	v string
}

func (s *fixedVersion) Version() string { return s.v }

// Relations lists every member's relations once, in member order, and a relation two members serve
// with different schemas is refused, naming both.
func TestNewFederationUnionsRelationsAndRefusesAConflict(t *testing.T) {
	a := NewMemSource().Declare("node", "id").Declare("edge", "src", "dst")
	b := NewMemSource().Declare("attr", "id", "key").Declare("node", "id")
	f, err := NewFederation(Member{Source: a}, Member{Source: b})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Relations(); !reflect.DeepEqual(got, []string{"edge", "node", "attr"}) {
		t.Errorf("relations %v", got)
	}
	for _, c := range []*MemSource{
		NewMemSource().Declare("node", "id", "kind"),
		NewMemSource().Declare("node", "name"),
		NewMemSource().DeclareSchema("node", Schema{Arity: 1, Labels: []string{"id"}, Types: []ArgType{{Type: "number"}}}),
	} {
		_, err := NewFederation(Member{Name: "repo", Source: a}, Member{Source: c})
		if err == nil || err.Error() != `query: federation: node has a different schema in member #1 than in member "repo"` {
			t.Errorf("err %v", err)
		}
	}
	inner, _ := NewFederation(Member{Source: a})
	outer, _ := NewFederation(Member{Source: inner})
	if outer.LooksUp("node") {
		t.Error("a federation of MemSources looks node up")
	}
	if _, err := NewFederation(Member{Source: a}, Member{Source: NewMemSource().DeclareSchema("node", Schema{Arity: 1, Labels: []string{"id"}, Doc: "other"})}); err != nil {
		t.Errorf("a different Doc is refused: %v", err)
	}
}

// The version changes when any member's does, and two different splits of versions never read alike.
func TestAFederationsVersionComposesItsMembers(t *testing.T) {
	x := &fixedVersion{MemSource: NewMemSource(), v: "a"}
	y := &fixedVersion{MemSource: NewMemSource(), v: "bc"}
	f, _ := NewFederation(Member{Source: x}, Member{Source: NewMemSource()}, Member{Source: y})
	before := f.Version()
	y.v = "bd"
	if f.Version() == before {
		t.Error("a member's version moved and the federation's didn't")
	}
	x.v, y.v = "ab", "c"
	g, _ := NewFederation(Member{Source: x}, Member{Source: NewMemSource()}, Member{Source: y})
	if g.Version() == before {
		t.Errorf("%q and %q split differently but read alike", "a|bc", "ab|c")
	}
}

// HoldsPrefix refuses a call binding a named position to a value without the prefix, and holds
// anything else.
func TestHoldsPrefix(t *testing.T) {
	h := HoldsPrefix("repo1/", map[string][]int{"edge": {0}})
	for _, c := range []struct {
		rel   string
		bound map[int]Value
		want  bool
	}{
		{"edge", map[int]Value{0: S("repo1/x")}, true},
		{"edge", map[int]Value{0: S("repo2/x")}, false},
		{"edge", map[int]Value{1: S("repo2/x")}, true},
		{"node", map[int]Value{0: S("repo2/x")}, true},
	} {
		if got := h(c.rel, c.bound); got != c.want {
			t.Errorf("%s %v: %v", c.rel, c.bound, got)
		}
	}
}

// A named member's tuples cite it, whole and looked up; an unnamed member's tuples are its own.
func TestAFederationNamesItsMembersInCitations(t *testing.T) {
	a := NewMemSource().Declare("node", "id")
	a.Add("node", Tuple{Vals: []Value{S("x")}, Cites: []string{"f.go:1"}})
	b := NewMemSource().Declare("node", "id")
	b.Add("node", Tuple{Vals: []Value{S("y")}, Cites: []string{"g.go:2"}})
	f, _ := NewFederation(Member{Name: "left", Source: a}, Member{Source: b})
	var cites []string
	for _, t := range f.Tuples("node") {
		cites = append(cites, t.Cites...)
	}
	if got := strings.Join(cites, " "); got != "left:f.go:1 g.go:2" {
		t.Errorf("cites %q", got)
	}
	if a.Tuples("node")[0].Cites[0] != "f.go:1" {
		t.Error("naming a member rewrote its Source's own tuples")
	}
}
