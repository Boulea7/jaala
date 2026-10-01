package stdlib

import "github.com/panyam/jaala/ns"

// Absent registers absent(?x) at the root, the only way to ASK about a field the source did not state.
// Before Value.Absent existed such a field bound to the empty string, so it was not merely hard to
// select, it was indistinguishable from one that was stated as "". Its negation is the useful half as
// often as not: `not absent(?min)` reads "this row states a lower bound". It is not a string test,
// since it asks about any value, which is why it is not under str.
func Absent(v *ns.Vocabulary) error {
	return v.AddPredicate("absent", ns.Builtin{
		Arity: 1, Labels: []string{"value"},
		Doc:   "reports whether the field carried no value at all, which is different from an empty string and from zero; `not absent(?x)` reads \"this field is stated\"",
		Holds: func(args []ns.Value) (bool, error) { return args[0].Absent, nil },
	})
}
