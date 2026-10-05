package stdlib

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/panyam/jaala/ns"
)

// Strings registers the string predicates under str: the tests str.contains, str.prefix and
// str.suffix, the two pattern tests str.glob (see CompileGlob) and str.match (see CompilePattern), and
// str.distance, which binds the edit distance between two strings.
func Strings(v *ns.Vocabulary) error {
	for _, p := range []struct {
		path string
		b    ns.Builtin
	}{
		{"str.contains", strFilter(strings.Contains, "substring", "reports whether a string contains a substring")},
		{"str.prefix", strFilter(strings.HasPrefix, "prefix", "reports whether a string starts with a prefix")},
		{"str.suffix", strFilter(strings.HasSuffix, "suffix", "reports whether a string ends with a suffix")},
		{"str.glob", patFilter(CompileGlob, "pattern", "reports whether the whole string matches a SQLite-style glob (`*` any run, `?` one character, `[a-z]` or `[^a-z]` one of a class, `[[]` a literal `[`)")},
		{"str.match", patFilter(CompilePattern, "regex", "reports whether the string matches an unanchored regular expression")},
		{"str.distance", strDistance()},
	} {
		if err := v.AddPredicate(p.path, p.b); err != nil {
			return err
		}
	}
	return nil
}

// strFilter wraps a string(value, pattern) bool as a 2-arity filter (the shape of
// str.contains/str.prefix/str.suffix).
func strFilter(fn func(s, pat string) bool, second, doc string) ns.Builtin {
	b := ns.Filter(2, func(args []ns.Value) (bool, error) { return fn(args[0].S, args[1].S), nil })
	return stringTest(b, second, doc)
}

// stringTest labels a two-argument string test and types both arguments as strings.
func stringTest(b ns.Builtin, second, doc string) ns.Builtin {
	b.Labels = []string{"string", second}
	b.Types = []ns.ArgType{{Type: ns.TypeString}, {Type: ns.TypeString}}
	b.Doc = doc
	return b
}

// patFilter is strFilter for the two PATTERN predicates (str.glob, str.match): the pattern must be compiled
// before it can be tested, so a malformed one is an EVAL ERROR rather than a non-match. That
// direction matters — a bad pattern that quietly matched nothing would read as "the data is clean"
// on a completeness check.
func patFilter(compile func(string) (*regexp.Regexp, error), second, doc string) ns.Builtin {
	return stringTest(ns.Filter(2, func(args []ns.Value) (bool, error) {
		re, err := compile(args[1].S)
		if err != nil {
			return false, err
		}
		return re.MatchString(args[0].S), nil
	}), second, doc)
}

// strDistance is str.distance(a, b, d): d is the Levenshtein distance between a and b, counted in
// characters (runes), so a non-ASCII letter is one edit. It needs both strings bound (#77), since
// enumerating the strings within a distance of another is not a finite answer. A planner waits for
// that, but a body run as written can call it early, which is an error as it is for a filter. With d
// bound too the engine unifies the emitted distance against it, so it tests.
func strDistance() ns.Builtin {
	return ns.Builtin{
		Arity:  3,
		Modes:  [][]bool{{true, true, false}},
		Labels: []string{"string", "other", "distance"},
		Types:  []ns.ArgType{{Type: ns.TypeString}, {Type: ns.TypeString}, {Type: ns.TypeNumber}},
		Doc:    "binds the edit (Levenshtein) distance between two strings, in characters; both strings must be bound",
		Gen: func(_ context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			if !args[0].Bound || !args[1].Bound {
				return errors.New("query: str.distance needs both strings bound (a variable must appear in a relation before str.distance measures it)")
			}
			a, b := args[0].Value, args[1].Value
			return emit([]ns.Value{a, b, ns.N(float64(levenshtein(a.S, b.S)))}, nil)
		},
	}
}

// levenshtein is the fewest single-character insertions, deletions and substitutions that turn a
// into b, over runes, keeping one row of the table.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	row := make([]int, len(rb)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		diag := row[0]
		row[0] = i
		for j := 1; j <= len(rb); j++ {
			up := row[j]
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			row[j] = min(row[j]+1, row[j-1]+1, diag+cost)
			diag = up
		}
	}
	return row[len(rb)]
}
