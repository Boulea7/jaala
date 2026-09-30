package datalog

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// An ArgType says what one argument of a relation denotes. The zero value says nothing: an
// argument the relation has not described.
//
// The engine never interprets a kind. "net" and "component" are opaque strings a host defines and
// reads back, typically to make an answer cell clickable. What the engine does is carry them: from
// a base relation or predicate that declares them, through the rules that read it, to the columns of
// an answer (see ColumnKinds), so a derived relation's arguments keep the kinds of the facts it was
// built from.
//
// An argument is described in one of three ways, plus an optional vocabulary:
//
//   - a fixed entity Kind: every value is a net;
//   - a kind taken from another argument per row (KindFrom): entity(name, kind) says in each row
//     what that row's name is;
//   - a Kind located through another argument (Owner): a pin names nothing on its own and is found
//     through the component in its owner argument;
//
// or, for a value that is not an entity, a scalar Type with its Unit.
type ArgType struct {
	// Kind is the entity kind every value of the argument names, or "" for none.
	Kind string
	// KindFrom is the label of the argument whose value, in each row, is this one's kind. When it is
	// set, Kind is "".
	KindFrom string
	// Owner is the label of the argument that locates this one, set beside Kind for an entity that is
	// only found through another, such as a pin through its component.
	Owner string
	// Type is a scalar type, TypeString or TypeNumber, for an argument that is not an entity.
	Type string
	// Unit is the base unit of a numeric argument ("V", "A"), for a reader; see Value.BaseUnit.
	Unit string
	// Domain closes the argument over a vocabulary. On a base relation or a derived one, a query
	// constant outside it is refused before evaluation. On a KindFrom argument it is also the set of
	// kinds a constant may name, so entity(?n, "net") types ?n as a net.
	Domain []string
}

// The scalar types an ArgType may name.
const (
	TypeString = "string"
	TypeNumber = "number"
)

// IsZero reports whether the type says nothing about its argument.
func (t ArgType) IsZero() bool {
	return t.Kind == "" && t.KindFrom == "" && t.Owner == "" && t.Type == "" && t.Unit == "" && len(t.Domain) == 0
}

// String renders the type in the syntax a rule head declares it with, without the leading ": ".
func (t ArgType) String() string {
	var s string
	switch {
	case t.KindFrom != "":
		s = "?" + t.KindFrom
	case t.Kind != "" && t.Owner != "":
		s = fmt.Sprintf("%s(?%s)", t.Kind, t.Owner)
	case t.Kind != "":
		s = t.Kind
	case t.Type == TypeNumber && t.Unit != "":
		s = fmt.Sprintf("number[%s]", t.Unit)
	case t.Type != "" && len(t.Domain) == 0:
		s = t.Type
	}
	if len(t.Domain) > 0 {
		q := make([]string, len(t.Domain))
		for i, d := range t.Domain {
			q[i] = fmt.Sprintf("%q", d)
		}
		dom := "{" + strings.Join(q, ", ") + "}"
		if s == "" {
			return dom
		}
		return s + " " + dom
	}
	return s
}

// sameShape reports whether two types say the same thing, ignoring Domain, which inference merges
// separately.
func (t ArgType) sameShape(o ArgType) bool {
	return t.Kind == o.Kind && t.KindFrom == o.KindFrom && t.Owner == o.Owner && t.Type == o.Type && t.Unit == o.Unit
}

// An ArgSig is one argument of a signature: its name and type, and whether the type was inferred
// from rules rather than declared.
type ArgSig struct {
	Name string
	ArgType
	// Inferred is set on a derived relation's argument whose type no rule head declares, so a host
	// can show a reader which types the author stated and which the engine worked out.
	Inferred bool
}

// String renders the argument as a rule head would declare it: "n: net", or "n" when nothing is
// known about it.
func (a ArgSig) String() string {
	if t := a.ArgType.String(); t != "" {
		return a.Name + ": " + t
	}
	return a.Name
}

// parseArgType reads a declaration, the text after ":" in a rule head argument:
//
//	decl   = "?" ident                       (* kind from that argument, per row *)
//	       | "string" | "number" [ "[" unit "]" ]
//	       | ident [ "(" "?" ident ")" ]     (* entity kind, optionally located by an owner *)
//	       | vocab ;
//	vocab  = "{" string { "," string } "}" ;  (* may also follow any of the above *)
//
// "string" and "number" are therefore not available as entity kinds.
func parseArgType(s string) (ArgType, error) {
	s = strings.TrimSpace(s)
	var t ArgType
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
			t.Type = TypeString
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
	case s == TypeString || s == TypeNumber:
		t.Type = s
	case strings.HasPrefix(s, TypeNumber+"[") && strings.HasSuffix(s, "]"):
		t.Type, t.Unit = TypeNumber, strings.TrimSpace(s[len(TypeNumber)+1:len(s)-1])
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
