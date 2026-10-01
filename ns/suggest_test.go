package ns

import (
	"strings"
	"testing"
)

// moved is a vocabulary just after its names moved into modules, which is when people type the old
// spellings.
func moved(t *testing.T) *Vocabulary {
	t.Helper()
	src := NewMemSource().
		Declare("edge", "from", "to").
		Declare("rails", "n").
		Declare("net_total", "n").
		Declare("net.total", "n").
		Declare("net.rail", "net").
		Declare("board.rail", "net").
		Declare("net.pin_count", "net", "n").
		Declare("component.net", "ref", "net").
		Declare("component.pin", "ref", "pin").
		Declare("pin.name", "ref", "pin", "name").
		Declare("pin.net", "ref", "pin", "net").
		Declare("x.a", "v").Declare("y.a", "v").Declare("z.a", "v")
	v := MustVocabulary(src)
	if err := v.AddLanguage(stub{new(int)}); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestABareNameSuggestsTheMembersItEnds(t *testing.T) {
	v := moved(t)
	for name, want := range map[string]string{
		"pin_count": `; did you mean "net.pin_count"?`,
		// rails at the root is one edit away, but members ending in rail are likelier meant.
		"rail":  `; "board.rail" and "net.rail" both end in "rail"`,
		"a":     `; "x.a", "y.a" and "z.a" all end in "a"`,
		"total": `; did you mean "net.total"?`,
		// Nothing ends in egde, so the root typo stands.
		"egde": `; did you mean "edge"?`,
	} {
		if got := v.Hint(name); got != want {
			t.Errorf("Hint(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAModuleNameSuggestsTheMemberItEnds(t *testing.T) {
	want := `"pin" is a module, not a relation; it holds name, net; did you mean "component.pin"?`
	if got := moved(t).Unknown("pin"); got != want {
		t.Errorf("Unknown(pin) = %q, want %q", got, want)
	}
}

func TestAPrivateMemberIsNeverSuggested(t *testing.T) {
	v := moved(t)
	if err := v.AddModule("m", "stub", "_secret/1", ""); err != nil {
		t.Fatal(err)
	}
	if got := v.Hint("_secret"); strings.Contains(got, "m._secret") {
		t.Errorf("Hint(_secret) = %q, which names a private member", got)
	}
}

func TestAPathSpelledWithAnotherSeparatorIsSuggested(t *testing.T) {
	v := moved(t)
	for name, want := range map[string]string{
		"net_pin_count":    `; did you mean "net.pin_count"?`,
		"net-pin-count":    `; did you mean "net.pin_count"?`,
		"component_on_net": `; did you mean "component.net"?`,
		// A close root name is the likelier typo and keeps its suggestion.
		"net_totl": `; did you mean "net_total"?`,
		"zzz_qqq":  "",
	} {
		if got := v.Hint(name); got != want {
			t.Errorf("Hint(%q) = %q, want %q", name, got, want)
		}
	}
}

// The docs a host shows for the standard predicates make claims a reader writes patterns by, so the
// claims are held to the behaviour (#17).
func TestStandardPredicateDocsSayWhatTheyDo(t *testing.T) {
	v := MustVocabulary(nil)
	if err := StandardPredicates(v); err != nil {
		t.Fatal(err)
	}
	holds := func(path string, args ...Value) bool {
		t.Helper()
		b, _ := v.Predicate(path)
		ok, err := b.Holds(args)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for _, c := range []struct {
		path, doc string
		cases     map[bool][][]Value
	}{
		{"str.glob", "the whole string matches a SQLite-style glob (`*` any run, `?` one character, `[a-z]` or `[^a-z]` one of a class, `[[]` a literal `[`)", map[bool][][]Value{
			true:  {{S("abc"), S("a?c")}, {S("abbbc"), S("a*c")}, {S("Bx"), S("[A-C]x")}, {S("Dx"), S("[^A-C]x")}, {S("D[1]"), S("D[[]1]")}},
			false: {{S("xabc"), S("a*")}, {S("abcx"), S("a?c")}, {S("abbc"), S("a?c")}},
		}},
		{"str.match", "an unanchored regular expression", map[bool][][]Value{
			true: {{S("xabcx"), S("ab")}},
		}},
		{"absent", "different from an empty string and from zero", map[bool][][]Value{
			true:  {{Absent()}},
			false: {{S("")}, {N(0)}},
		}},
	} {
		b, _ := v.Predicate(c.path)
		if !strings.Contains(b.Doc, c.doc) {
			t.Errorf("%s doc = %q, want it to say %q", c.path, b.Doc, c.doc)
		}
		for want, argsets := range c.cases {
			for _, args := range argsets {
				if got := holds(c.path, args...); got != want {
					t.Errorf("%s%v = %v, want %v", c.path, args, got, want)
				}
			}
		}
	}
}
