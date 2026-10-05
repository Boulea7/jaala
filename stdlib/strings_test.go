package stdlib

import (
	"context"
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

func TestLevenshteinCountsCharacterEdits(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"GND", "GND", 0},
		{"", "abc", 3},
		{"PMIC_EN", "PMIC_EM", 1},
		{"SDA0", "SDAO", 1},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"R1Ω", "R2Ω", 1},
		{"Ω", "O", 1},
	} {
		for _, p := range [][2]string{{c.a, c.b}, {c.b, c.a}} {
			if got := levenshtein(p[0], p[1]); got != c.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", p[0], p[1], got, c.want)
			}
		}
	}
}

func TestDistanceNeedsBothStringsAndBindsTheThird(t *testing.T) {
	v := ns.MustVocabulary(nil)
	if err := Register(v); err != nil {
		t.Fatal(err)
	}
	b, ok := v.Predicate("str.distance")
	if !ok || !b.IsGenerator() {
		t.Fatalf("str.distance = %+v, %v, want a generator", b, ok)
	}
	if !b.Satisfied([]bool{true, true, false}) || b.Satisfied([]bool{true, false, true}) || b.Satisfied([]bool{false, true, true}) {
		t.Errorf("str.distance modes = %v, want both strings bound and the distance free", b.Modes)
	}
	var got []ns.Value
	err := b.Gen(context.Background(), nil, []ns.Arg{{Value: ns.S("PMIC_EN"), Bound: true}, {Value: ns.S("PMIC_EM"), Bound: true}, {}}, func(vals []ns.Value, _ []string) error {
		got = append(got, vals...)
		return nil
	})
	if err != nil || len(got) != 3 || got[0].S != "PMIC_EN" || got[1].S != "PMIC_EM" || got[2].Num == nil || *got[2].Num != 1 {
		t.Errorf("str.distance(PMIC_EN, PMIC_EM, ?d) emitted %v (%v), want one row with d = 1", got, err)
	}
	if err := b.Gen(context.Background(), nil, []ns.Arg{{Value: ns.S("PMIC_EN"), Bound: true}, {}, {}}, func([]ns.Value, []string) error {
		t.Error("str.distance emitted with its second string unbound")
		return nil
	}); err == nil || !strings.HasPrefix(err.Error(), "query: ") {
		t.Errorf("str.distance with an unbound string: err = %v, want a query: error", err)
	}
}
