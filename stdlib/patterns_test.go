package stdlib

import (
	"strings"
	"testing"
)

// Declaire's own table (query/render_test.go, TestGlob), so the two engines' globs stay in step, plus
// what SQLite GLOB adds to it: classes, ranges, negation, a literal "]" first, and "[[]" for "[".
func TestGlobFollowsSQLite(t *testing.T) {
	for _, c := range []struct {
		pattern, s string
		want       bool
	}{
		{"net/http.ServeMux.Handle*", "net/http.ServeMux.HandleFunc", true},
		{"*/likes*", "/{notationId}/likes/", true},
		{"*likes", "/likes/", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"[AB]x", "Bx", true},
		{"[^AB]x", "Bx", false},
		{"[a-c]", "b", true},
		{"a.b", "axb", false},
		{"A*", "a", false},
		{"/amp1/*", "/amp1/sub/DATA0", true}, // * crosses /
		{"[^a-c]", "d", true},
		{"[a-cx-z]", "y", true},
		{"[a-cx-z]", "m", false},
		{"[]x]", "]", true},   // ] first is literal
		{"[^]x]", "]", false}, // and after ^
		{"[-a]", "-", true},   // - with nothing before it is literal
		{"[a-]", "-", true},   // and with nothing after
		{"[*]", "*", true},    // a class holds a literal *
		{"[*]", "x", false},
		{"DATA[[]1:0]", "DATA[1:0]", true}, // the literal-bracket spelling
		{"DATA[1:0]", "DATA[1:0]", false},  // which plain brackets no longer are
		{"DATA[1:0]", "DATA1", true},
		{"a+b(c)", "a+b(c)", true}, // regexp metacharacters stay literal
		{"\\[x]", "\\x", true},     // no escape character: \\ is a literal backslash
		{"a\nb*", "a\nbc", true},
		{"a*", "a\nb", true}, // * crosses a newline too, as SQLite's does
	} {
		re, err := CompileGlob(c.pattern)
		if err != nil {
			t.Errorf("CompileGlob(%q): %v", c.pattern, err)
			continue
		}
		if got := re.MatchString(c.s); got != c.want {
			t.Errorf("glob %q on %q = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

// A malformed class is an error, as a bad regex is, rather than a pattern that quietly matches
// nothing (Declaire's own Glob answers false here; str.glob refuses).
func TestABadGlobIsRefused(t *testing.T) {
	for pattern, frag := range map[string]string{
		"[unclosed": "unclosed [",
		"a[":        "unclosed [",
		"[]":        "unclosed [",
		"[^":        "unclosed [",
		"[z-a]":     "range z-a runs backwards",
	} {
		if _, err := CompileGlob(pattern); err == nil || !strings.Contains(err.Error(), frag) || !strings.Contains(err.Error(), "query: invalid glob") {
			t.Errorf("CompileGlob(%q): err = %v, want %q", pattern, err, frag)
		}
	}
}
