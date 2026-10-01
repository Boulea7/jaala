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
