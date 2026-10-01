package stdlib

import (
	"regexp"
	"strings"

	"github.com/panyam/jaala/ns"
)

// Strings registers the string tests under str: str.contains, str.prefix and str.suffix, and the two
// pattern tests str.glob (see CompileGlob) and str.match (see CompilePattern).
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
