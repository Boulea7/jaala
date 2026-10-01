package datalog

import (
	"fmt"
	"github.com/panyam/jaala/ns"
	"slices"
	"sort"
	"strings"
)

// parseArgType reads a declaration, the text after ":" in a rule head argument:
//
//	decl   = "?" ident                       (* kind from that argument, per row *)
//	       | "string" | "number" [ "[" unit "]" ]
//	       | ident [ "(" "?" ident ")" ]     (* entity kind, optionally located by an owner *)
//	       | vocab ;
//	vocab  = "{" string { "," string } "}" ;  (* may also follow any of the above *)
//
// "string" and "number" are therefore not available as entity kinds.
func parseArgType(s string) (ns.ArgType, error) {
	s = strings.TrimSpace(s)
	var t ns.ArgType
	if i := strings.IndexByte(s, '{'); i >= 0 {
		if !strings.HasSuffix(s, "}") {
			return t, fmt.Errorf("query: unterminated vocabulary in %q", s)
		}
		for _, item := range splitTop(s[i+1:len(s)-1], ",") {
			item = strings.TrimSpace(item)
			if len(item) < 2 || item[0] != '"' || item[len(item)-1] != '"' {
				return t, fmt.Errorf("query: vocabulary entry %q must be a \"string\"", item)
			}
			t.Domain = append(t.Domain, item[1:len(item)-1])
		}
		s = strings.TrimSpace(s[:i])
		if s == "" {
			t.Type = ns.TypeString
			return t, nil
		}
	}
	switch {
	case s == "":
		return t, fmt.Errorf("query: empty type declaration")
	case s[0] == '?':
		if !isIdent(s[1:]) {
			return t, fmt.Errorf("query: bad kind variable %q", s)
		}
		t.KindFrom = s[1:]
	case s == ns.TypeString || s == ns.TypeNumber:
		t.Type = s
	case strings.HasPrefix(s, ns.TypeNumber+"[") && strings.HasSuffix(s, "]"):
		t.Type, t.Unit = ns.TypeNumber, strings.TrimSpace(s[len(ns.TypeNumber)+1:len(s)-1])
	case strings.HasSuffix(s, ")"):
		open := strings.IndexByte(s, '(')
		owner := strings.TrimSpace(s[open+1 : len(s)-1])
		if open < 0 || len(owner) < 2 || owner[0] != '?' || !isIdent(owner[1:]) || !isIdent(strings.TrimSpace(s[:open])) {
			return t, fmt.Errorf("query: bad type declaration %q (an owner is written kind(?var))", s)
		}
		t.Kind, t.Owner = strings.TrimSpace(s[:open]), owner[1:]
	case isIdent(s):
		t.Kind = s
	default:
		return t, fmt.Errorf("query: bad type declaration %q", s)
	}
	return t, nil
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// unionSorted merges vocabularies, sorted and deduped.
func unionSorted(a, b []string) []string {
	out := append(append([]string(nil), a...), b...)
	sort.Strings(out)
	return slices.Compact(out)
}
