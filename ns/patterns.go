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

// CompileGlob compiles pattern as a shell-style glob, memoized: `*` matches any run of characters,
// `?` matches exactly one, and everything else is literal. Unlike CompilePattern the match is
// WHOLE-STRING, which is what a glob conventionally means.
//
// It translates to a regexp rather than deferring to path.Match because path.Match's `*` does not
// cross `/`, and hierarchical net names contain `/` (a sub-sheet local is `/amp1/DATA0`), so
// path.Match would silently under-match exactly the names a multi-instance interface is named with.
// The translation escapes every other character, so the result always compiles; the error return
// exists so callers can handle both pattern forms through one shape.
func CompileGlob(pattern string) (*regexp.Regexp, error) {
	re, err := compileCached("glob\x00"+pattern, globToRegexp(pattern))
	if err != nil {
		return nil, fmt.Errorf("query: invalid glob %q: %w", pattern, err)
	}
	return re, nil
}

// globToRegexp renders a glob as an anchored regexp source.
func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}
