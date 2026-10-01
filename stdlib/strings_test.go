package stdlib

import (
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

// The docs a host shows for the standard predicates make claims a reader writes patterns by, so the
// claims are held to the behaviour (#17).
func TestStandardPredicateDocsSayWhatTheyDo(t *testing.T) {
	v := ns.MustVocabulary(nil)
	if err := Register(v); err != nil {
		t.Fatal(err)
	}
	holds := func(path string, args ...ns.Value) bool {
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
		cases     map[bool][][]ns.Value
	}{
		{"str.glob", "the whole string matches a SQLite-style glob (`*` any run, `?` one character, `[a-z]` or `[^a-z]` one of a class, `[[]` a literal `[`)", map[bool][][]ns.Value{
			true:  {{ns.S("abc"), ns.S("a?c")}, {ns.S("abbbc"), ns.S("a*c")}, {ns.S("Bx"), ns.S("[A-C]x")}, {ns.S("Dx"), ns.S("[^A-C]x")}, {ns.S("D[1]"), ns.S("D[[]1]")}},
			false: {{ns.S("xabc"), ns.S("a*")}, {ns.S("abcx"), ns.S("a?c")}, {ns.S("abbc"), ns.S("a?c")}},
		}},
		{"str.match", "an unanchored regular expression", map[bool][][]ns.Value{
			true: {{ns.S("xabcx"), ns.S("ab")}},
		}},
		{"absent", "different from an empty string and from zero", map[bool][][]ns.Value{
			true:  {{ns.Absent()}},
			false: {{ns.S("")}, {ns.N(0)}},
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
