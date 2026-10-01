package ns

import (
	"fmt"
	"sort"
	"strings"
)

// An EntryKind is what sits at a path in the tree.
type EntryKind string

// The things a path can name.
const (
	EntryModule    EntryKind = "module"
	EntryBase      EntryKind = "base"
	EntryPredicate EntryKind = "predicate"
	EntryDerived   EntryKind = "derived"
)

// An Entry describes what sits at one path, for a host offering drill-down discovery: list a module,
// pick a member, read its definition.
type Entry struct {
	Path string
	Kind EntryKind
	// Args is a member's signature. On a derived member, an argument no rule head declares is
	// inferred and marked so.
	Args []ArgSig
	// Doc is the member's description: a Schema's or Builtin's Doc, or for a derived member the
	// comment lines above its first rule.
	Doc string
	// Module and Definition locate a derived member's definition: the path of the module defining it
	// and its clauses as that module's language reports them.
	Module     string
	Definition []string
	// Members are a module's direct children as full paths, sorted. A child may itself be a module.
	Members []string
}

// Signature renders a member as a rule head declaring its types: `net.has_test_point(n: net)`.
func (e Entry) Signature() string {
	args := make([]string, len(e.Args))
	for i, a := range e.Args {
		args[i] = a.String()
	}
	return e.Path + "(" + strings.Join(args, ", ") + ")"
}

// Lookup answers what is at path: a module (its members) or a member (its signature, kind, doc, and
// for a derived member its definition). "" is the root module. It runs Check first, since a derived
// member's signature is only known once every module has been resolved, and returns Check's error
// when the modules do not check. An unknown path is an error worded as the engine words it in a
// query, suggestion included.
func (v *Vocabulary) Lookup(path string) (Entry, error) {
	sigs, err := v.signatures()
	if err != nil {
		return Entry{}, err
	}
	if path == "" || v.IsModule(path) {
		children := v.children(path)
		out := Entry{Path: path, Kind: EntryModule, Members: make([]string, len(children))}
		for i, c := range children {
			out.Members[i] = joinPath(path, c)
		}
		return out, nil
	}
	m, ok := v.members[path]
	if !ok {
		return Entry{}, fmt.Errorf("query: %s", v.Unknown(path))
	}
	switch m.kind {
	case kindBase:
		return Entry{Path: path, Kind: EntryBase, Args: sigOf(m.schema.Labels, m.schema.Types, m.schema.Arity), Doc: m.schema.Doc}, nil
	case kindPredicate:
		return Entry{Path: path, Kind: EntryPredicate, Args: sigOf(m.pred.Labels, m.pred.Types, m.pred.Arity), Doc: m.pred.Doc}, nil
	}
	mod := v.mods[m.module]
	_, leaf := splitPath(path)
	e := Entry{Path: path, Kind: EntryDerived, Args: sigs[path], Module: mod.Path}
	for _, d := range mod.Members {
		if d.Name == leaf {
			e.Doc, e.Definition = d.Doc, d.Definition
		}
	}
	return e, nil
}

// Members lists what a module holds, one Entry per direct child, sorted by path. "" is the root.
func (v *Vocabulary) Members(module string) ([]Entry, error) {
	e, err := v.Lookup(module)
	if err != nil {
		return nil, err
	}
	if e.Kind != EntryModule {
		return nil, fmt.Errorf("query: %q is a %s, not a module", module, e.Kind)
	}
	out := make([]Entry, 0, len(e.Members))
	for _, p := range e.Members {
		c, err := v.Lookup(p)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// sigOf zips labels and types into a signature of the given arity. An unlabelled argument is named
// argN.
func sigOf(labels []string, types []ArgType, arity int) []ArgSig {
	n := max(arity, len(labels), len(types))
	out := make([]ArgSig, n)
	for i := range out {
		out[i].Name = fmt.Sprintf("arg%d", i)
		if i < len(labels) && labels[i] != "" {
			out[i].Name = labels[i]
		}
		if i < len(types) {
			out[i].ArgType = types[i]
		}
	}
	return out
}
