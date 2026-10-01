package ns

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// This file is the single compiler for the two PATTERN predicates, glob and match. It is exported
// because a caller that also needs to evaluate the same pattern in Go — the profiles package matches
// nets to signals both in generated datalog and in its Go presence gates (a twin discipline: one compiler, two callers)
// — must not re-implement the translation. Sharing the compiler means the Go side and the datalog
// side cannot disagree by construction, which a hand-written twin could.

// patternCache memoizes compiled patterns by form-qualified source string. Matching runs a pattern
// over every net of a design, once per signal, so recompiling per row is pure waste; the cache is
// keyed by the SOURCE text (not the translated regex) so a glob and a regex that happen to translate
// alike never collide. Failures are cached too, so a bad pattern costs one compile, not one per row.
var patternCache sync.Map // string -> compiledPattern

type compiledPattern struct {
	re  *regexp.Regexp
	err error
}

func compileCached(key, expr string) (*regexp.Regexp, error) {
	if v, ok := patternCache.Load(key); ok {
		c := v.(compiledPattern)
		return c.re, c.err
	}
	re, err := regexp.Compile(expr)
	patternCache.Store(key, compiledPattern{re: re, err: err})
	return re, err
}

// CompilePattern compiles pattern as an RE2 regular expression, memoized. The match is UNANCHORED
// (Go's regexp semantics): `_H` matches any name containing `_H`, so a caller that means "the whole
// name" writes `^...$`. That is deliberate — a regex predicate that silently anchored would surprise
// anyone who knows RE2, and anchoring is one character. An invalid pattern is an error here rather
// than a silent non-match, so a typo surfaces instead of reading as "nothing matched".
func CompilePattern(pattern string) (*regexp.Regexp, error) {
	re, err := compileCached("regex\x00"+pattern, pattern)
	if err != nil {
		return nil, fmt.Errorf("query: invalid regex %q: %w", pattern, err)
	}
	return re, nil
}

// CompileGlob compiles pattern as a glob with SQLite GLOB's semantics, memoized: `*` matches any run
// of characters, `?` exactly one, and `[...]` one character of a class (`[abc]`, `[a-z]`, `[^abc]`).
// Unlike CompilePattern the match is WHOLE-STRING and case-sensitive, which is what a glob
// conventionally means. A literal `[` is written `[[]`, so a bus name like DATA[1:0] is matched as
// `DATA[[]1:0]`. An unclosed `[` is an error rather than a pattern that matches nothing.
//
// It translates to a regexp rather than deferring to path.Match because path.Match's `*` does not
// cross `/`, and hierarchical net names contain `/` (a sub-sheet local is `/amp1/DATA0`), so
// path.Match would silently under-match exactly the names a multi-instance interface is named with.
// SQLite's dialect is the one Declaire's walk selectors use, so a pattern means the same in a host's
// SQL and in a query.
func CompileGlob(pattern string) (*regexp.Regexp, error) {
	expr, err := globToRegexp(pattern)
	if err == nil {
		var re *regexp.Regexp
		if re, err = compileCached("glob\x00"+pattern, expr); err == nil {
			return re, nil
		}
	}
	return nil, fmt.Errorf("query: invalid glob %q: %w", pattern, err)
}

// globToRegexp translates a glob with SQLite GLOB's semantics, case-sensitive and matching the whole
// string: "*" any run of characters ("/" included, which hierarchical net names rely on), "?" any one,
// and "[...]" one character from a class. In a class, "a-z" is a range, a leading "^" negates, and a
// "]" first (after any "^") is literal, so "[]x]" holds "]" and "x". There is no escape character;
// a literal "[" is written "[[]", and a literal "*" "[*]". Everything else is literal.
func globToRegexp(pattern string) (string, error) {
	var b strings.Builder
	b.WriteString("(?s)^")
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			class, next, err := globClass(rs, i+1)
			if err != nil {
				return "", err
			}
			b.WriteString(class)
			i = next
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String(), nil
}

// globClass translates the class starting after a "[" at rs[i], returning the regexp class and the
// index of its closing "]".
func globClass(rs []rune, i int) (string, int, error) {
	var b strings.Builder
	b.WriteString("[")
	if i < len(rs) && rs[i] == '^' {
		b.WriteString("^")
		i++
	}
	lit := func(r rune) string { return fmt.Sprintf(`\x{%x}`, r) }
	for first := true; ; first = false {
		if i >= len(rs) {
			return "", 0, fmt.Errorf("unclosed [ (write [[] for a literal [)")
		}
		r := rs[i]
		if r == ']' && !first {
			b.WriteString("]")
			return b.String(), i, nil
		}
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != ']' {
			if rs[i+2] < r {
				return "", 0, fmt.Errorf("range %c-%c runs backwards", r, rs[i+2])
			}
			b.WriteString(lit(r) + "-" + lit(rs[i+2]))
			i += 3
			continue
		}
		b.WriteString(lit(r))
		i++
	}
}
