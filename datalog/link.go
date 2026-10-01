package datalog

import (
	"fmt"
	"github.com/panyam/jaala/ns"
	"strings"
)

// Link returns q with every derived module it reaches expanded into its rules, so the result is an
// ordinary query that evaluates without the vocabulary's modules. Naive.Eval, Validate, Reads and
// GeneratorFirstRules link first; a host calls Link itself to inspect or cache the expanded program.
//
// Loading is transitive and by need: a query naming net.has_test_point pulls in the module call that
// registered it, then whatever that module's rules read, and so on. Each call is loaded once, however
// many paths reach it. Module rules come after the query's own, heads renamed to full paths and
// private members to names no query can spell. Because the result is ordinary rules, stratification,
// negation safety and range restriction apply to module rules exactly as to a query's own.
//
// A query's own rules stay local and bare. Link refuses a query rule whose head is a qualified path
// (quietly adding a rule to a shared relation is the accident a namespace exists to prevent), names a
// module, or names a derived relation a module registered. It returns Check's error when the vocabulary's
// modules do not check.
func Link(q Query, reg *ns.Vocabulary) (Query, error) {
	for _, r := range q.Rules {
		head := r.Head.Relation
		switch {
		case strings.Contains(head, "."):
			return Query{}, fmt.Errorf("query: rule head %q is a qualified path; a query defines only its own bare relations, and a shared one is registered with AddModule", head)
		case reg.IsModule(head):
			return Query{}, fmt.Errorf("query: rule head %q is a module, not a relation", head)
		}
		if id, ok := reg.DefiningModule(head); ok {
			return Query{}, fmt.Errorf("query: rule head %q redefines a derived relation registered in module %q", head, reg.Modules()[id].Path)
		}
	}
	var pending []string
	visit := func(b Body) error {
		for _, a := range ruleAtoms(b) {
			if strings.Contains(a.Relation, privateSep) {
				return fmt.Errorf("query: %s", reg.Unknown(displayName(a.Relation)))
			}
			pending = append(pending, a.Relation)
		}
		return nil
	}
	for _, r := range q.Rules {
		if err := visit(r.Body); err != nil {
			return Query{}, err
		}
	}
	if err := visit(q.Goal); err != nil {
		return Query{}, err
	}
	res, err := resolved(reg)
	if err != nil {
		return Query{}, err
	}
	loaded := map[int]bool{}
	var linked []Rule
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		id, ok := reg.DefiningModule(name)
		if !ok || loaded[id] {
			continue
		}
		loaded[id] = true
		for _, r := range res.rules[id] {
			linked = append(linked, r)
			for _, a := range ruleAtoms(r.Body) {
				pending = append(pending, a.Relation)
			}
		}
	}
	if len(linked) == 0 {
		return q, nil
	}
	out := q
	out.Rules = append(append([]Rule(nil), q.Rules...), linked...)
	return out, nil
}
