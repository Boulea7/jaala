package datalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A unit is one AddModule call: rules registered at a module path. It is the scope of its private
// members, so two units may each define an _helper without meeting.
type unit struct {
	module   string
	rules    []Rule // as written: heads bare, bodies unresolved
	privates map[string]bool
	docs     map[string]string // by bare member name, from the comment lines above its first rule
}

// AddModule registers derived relations, written as Datalog rules, in the module at path. Each rule
// head names a member of that module and is written bare: `has_test_point(?n) :- ...` registered at
// "net" defines net.has_test_point. A path of "" registers at the root.
//
// Inside the text, a bare name is looked up in this module first, among members of every kind and
// from any unit, and then at the root. A dotted name is always a full path. So in module "net",
// `pin_count(?n, ?c)` reads net.pin_count when that is registered, and `component.pin(...)` reads
// component.pin wherever it sits.
//
// A member whose name starts with "_" is private to this call: it is not registered at a path, no
// query can name it, and another call's _helper is a different relation.
//
// AddModule refuses text holding a goal, a qualified rule head, a member defined with two arities, and
// any member path the tree's rules refuse (see Registry). Nothing is registered when it fails. What it
// cannot check alone, because a name may be registered later, waits for Check: that every name the
// rules read exists, and that the rules are well formed together.
//
// Several calls may register at one module path, provided each member path has one definer, which is
// how a host's standard library and a project's own relations share a module.
//
// A rule head may declare its arguments' types, `has_test_point(?n: net)`; an argument no head
// declares is inferred (see Registry.Lookup). The comment lines directly above a member's first rule
// are its doc.
func (r *Registry) AddModule(path, text string) error {
	if path != "" {
		if err := checkPath(path); err != nil {
			return err
		}
	}
	rules, err := ParseRules(text)
	if err != nil {
		return fmt.Errorf("%w (in module %q)", err, path)
	}
	u := &unit{module: path, rules: rules, privates: map[string]bool{}, docs: moduleDocs(text)}
	arity := map[string]int{}
	var public []string
	for _, rule := range rules {
		name := rule.Head.Relation
		if strings.Contains(name, ".") {
			return fmt.Errorf("query: module %q rule head %q is qualified; a module defines its own members, written bare", path, name)
		}
		if prev, ok := arity[name]; ok {
			if prev != len(rule.Head.Args) {
				return fmt.Errorf("query: module %q defines %q with %d and %d args (arity must be consistent)", path, name, prev, len(rule.Head.Args))
			}
			continue
		}
		arity[name] = len(rule.Head.Args)
		if strings.HasPrefix(name, "_") {
			u.privates[name] = true
		} else {
			public = append(public, name)
		}
	}
	id := len(r.units)
	for _, name := range public {
		if err := r.admits(joinPath(path, name), member{kind: kindDerived, unit: id}); err != nil {
			return err
		}
	}
	for _, name := range public {
		r.put(joinPath(path, name), member{kind: kindDerived, unit: id})
	}
	r.units = append(r.units, u)
	r.check = &checkState{}
	return nil
}

// Check is the semantic pass over every registered module: it resolves each name the modules' rules
// read to a path, and validates all module rules together, as one program, against the registry.
// That catches a name no one registered, a wrong arity, an unbound head variable, unsafe negation and
// recursion through negation, including across modules, before any query runs.
//
// It runs lazily, because modules may read each other in any registration order: Link calls it, and
// the result is kept until something new is registered. A host calls it directly to report a broken
// library at load rather than at its first query. As elsewhere, a registry holding no base relation
// cannot call a name unknown, so that one check stands down until it has some.
func (r *Registry) Check() error {
	_, err := r.resolvedUnits()
	return err
}

// resolvedUnits returns every unit's rules with names resolved to paths, computing them once.
func (r *Registry) resolvedUnits() ([][]Rule, error) {
	if r == nil {
		return nil, nil
	}
	c := r.check
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.done {
		c.resolved, c.err = r.resolveAll()
		c.done = true
	}
	return c.resolved, c.err
}

func (r *Registry) resolveAll() ([][]Rule, error) {
	out := make([][]Rule, len(r.units))
	privates := map[string]bool{}
	var all []Rule
	for i, u := range r.units {
		for name := range u.privates {
			privates[privateName(u.module, name, i)] = true
		}
		for _, rule := range u.rules {
			out[i] = append(out[i], r.resolveRule(u, i, rule))
		}
		all = append(all, out[i]...)
	}
	if len(all) == 0 {
		return out, nil
	}
	if r.hasBase() {
		for i, rules := range out {
			for _, rule := range rules {
				for _, a := range ruleAtoms(rule.Body) {
					if privates[a.Relation] || r.isMember(a.Relation) {
						continue
					}
					return nil, fmt.Errorf("query: module %q rule %q reads %s", r.units[i].module, displayName(rule.Head.Relation), r.unknown(a.Relation))
				}
			}
		}
	}
	b := newValidationBase(r)
	if _, _, err := b.checkRules(all); err != nil {
		return nil, err
	}
	t := newTyper(r, all)
	sigs := map[string][]ArgSig{}
	for path, m := range r.members {
		if m.kind != kindDerived {
			continue
		}
		sig, err := t.signature(path)
		if err != nil {
			return nil, err
		}
		sigs[path] = sig
	}
	r.check.sigs = sigs
	if r.hasBase() {
		for _, rule := range all {
			if err := b.checkLiterals(rule.Body.Literals); err != nil {
				return nil, fmt.Errorf("%w (in module rule %q)", err, displayName(rule.Head.Relation))
			}
			_, negs := splitNegations(rule.Body.Literals)
			if err := b.validateNegations(rule.Body, negs); err != nil {
				return nil, fmt.Errorf("%w (in module rule %q)", err, displayName(rule.Head.Relation))
			}
		}
	}
	return out, nil
}

// resolveRule rewrites one rule of unit u so every relation it names is a full path: its head the
// member it defines, and each body name by the lookup AddModule describes.
func (r *Registry) resolveRule(u *unit, id int, rule Rule) Rule {
	name := func(n string) string {
		switch {
		case strings.Contains(n, "."):
			return n
		case u.privates[n]:
			return privateName(u.module, n, id)
		}
		if p := joinPath(u.module, n); u.module != "" && r.isMember(p) {
			return p
		}
		return n
	}
	out := Rule{Head: renameAtom(rule.Head, name), Hops: rule.Hops, HeadTypes: rule.HeadTypes}
	for _, lit := range rule.Body.Literals {
		switch {
		case lit.Pos != nil:
			a := renameAtom(*lit.Pos, name)
			out.Body.Literals = append(out.Body.Literals, Literal{Pos: &a})
		case lit.Neg != nil:
			a := renameAtom(*lit.Neg, name)
			out.Body.Literals = append(out.Body.Literals, Literal{Neg: &a})
		default:
			out.Body.Literals = append(out.Body.Literals, lit)
		}
	}
	return out
}

func renameAtom(a Atom, name func(string) string) Atom {
	return Atom{Relation: name(a.Relation), Args: a.Args}
}

// ruleAtoms returns the atoms of a body, positive and negated.
func ruleAtoms(b Body) []*Atom {
	var out []*Atom
	for _, l := range b.Literals {
		if l.Pos != nil {
			out = append(out, l.Pos)
		}
		if l.Neg != nil {
			out = append(out, l.Neg)
		}
	}
	return out
}

func (r *Registry) isMember(path string) bool {
	_, ok := r.members[path]
	return ok
}

// privateSep marks a private member's linked name. The parser never accepts it in a relation name,
// so no query text can spell a private member, and the unit index keeps two units' privates apart.
const privateSep = "~"

func privateName(module, name string, id int) string {
	return joinPath(module, name) + privateSep + strconv.Itoa(id)
}

// displayName is a linked relation name as its author wrote it: a private member loses its unit
// suffix, so an error or a lint names `net._probe_count` rather than the linker's spelling.
func displayName(rel string) string {
	if i := strings.Index(rel, privateSep); i >= 0 {
		return rel[:i]
	}
	return rel
}

func joinPath(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}

// displayNames maps displayName over names, sorted and deduped.
func displayNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		if d := displayName(n); !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// moduleDocs collects, for each member, the comment lines directly above the first rule that defines
// it. A blank line ends a comment block, so a file header is not mistaken for its first member's doc.
func moduleDocs(text string) map[string]string {
	docs := map[string]string{}
	var pending []string
	for _, line := range strings.Split(text, "\n") {
		tl := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(tl, "#"):
			pending = append(pending, strings.TrimSpace(tl[1:]))
			continue
		case tl == "":
			pending = nil
			continue
		}
		if open := strings.IndexByte(tl, '('); open > 0 && len(pending) > 0 {
			if name := strings.TrimSpace(tl[:open]); isRelation(name) {
				if _, ok := docs[name]; !ok {
					docs[name] = strings.Join(pending, " ")
				}
			}
		}
		pending = nil
	}
	return docs
}
