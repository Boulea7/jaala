package demo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/panyam/jaala/ns"
)

// Fact is one ground atom from a fact text, with the text it was written as, which is its citation.
type Fact struct {
	Relation string
	Args     []ns.Value
	Text     string
}

// ParseFacts reads ground atoms: rel("text", 3), one per line or separated by ";", with an optional
// trailing ".". A "#" outside a string starts a comment. Arguments are strings in double quotes or
// numbers. jaala has no fact syntax of its own, since its hosts serve facts from Go, so this is the
// docsite's.
func ParseFacts(text string) ([]Fact, error) {
	var out []Fact
	for n, line := range strings.Split(text, "\n") {
		for _, stmt := range splitOutsideStrings(stripComment(line), ';') {
			stmt = strings.TrimSuffix(strings.TrimSpace(stmt), ".")
			if stmt == "" {
				continue
			}
			f, err := parseFact(stmt)
			if err != nil {
				return nil, fmt.Errorf("demo: facts line %d: %v", n+1, err)
			}
			out = append(out, f)
		}
	}
	return out, nil
}

func parseFact(s string) (Fact, error) {
	open := strings.IndexByte(s, '(')
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return Fact{}, fmt.Errorf("%q isn't rel(arg, ...)", s)
	}
	f := Fact{Relation: strings.TrimSpace(s[:open])}
	for _, a := range splitOutsideStrings(s[open+1:len(s)-1], ',') {
		a = strings.TrimSpace(a)
		switch {
		case len(a) >= 2 && a[0] == '"' && a[len(a)-1] == '"':
			f.Args = append(f.Args, ns.S(a[1:len(a)-1]))
		default:
			x, err := strconv.ParseFloat(a, 64)
			if err != nil {
				return Fact{}, fmt.Errorf("argument %q of %q is neither a quoted string nor a number", a, s)
			}
			f.Args = append(f.Args, ns.N(x))
		}
	}
	args := make([]string, len(f.Args))
	for i, v := range f.Args {
		if v.Num != nil {
			args[i] = v.S
		} else {
			args[i] = strconv.Quote(v.S)
		}
	}
	f.Text = f.Relation + "(" + strings.Join(args, ", ") + ")"
	return f, nil
}

func stripComment(line string) string {
	in := false
	for i, r := range line {
		switch {
		case r == '"':
			in = !in
		case r == '#' && !in:
			return line[:i]
		}
	}
	return line
}

func splitOutsideStrings(s string, sep rune) []string {
	var out []string
	in, start := false, 0
	for i, r := range s {
		switch {
		case r == '"':
			in = !in
		case r == sep && !in:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
