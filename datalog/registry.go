package datalog

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// A Registry is the namespace tree a query's names resolve in. Every callable name lives at a PATH,
// its segments separated by ".": `edge`, `str.contains`, `acme.power.rail_budget`. A path's last
// segment is a member; the segments before it name the module holding it, and the root module ("")
// holds the members with no prefix.
//
// Two rules keep a path unambiguous, and the registry enforces both when a member is added:
//
//   - A segment is a module or a member, never both. `pin` beside `pin.net` is refused, because a
//     reader of `pin.net(...)` could not tell a member of module `pin` from a relation named with a
//     dot.
//   - One definer per path. Registering a path twice is an error naming both definitions, whatever
//     their kinds, so a host predicate cannot silently shadow a base relation or the reverse.
//
// A Registry is a value a host composes and hands to NewBase, not ambient state, so two hosts in one
// process can offer different vocabularies. It is not safe for concurrent registration, but once
// built it may be read by any number of Bases and Evals at once.
type Registry struct {
	src     Source
	members map[string]member
	// modules counts, per module path, how many members sit anywhere beneath it. A module exists
	// exactly while something is registered under it, which is what lets a registration refuse a
	// member whose path is already a module.
	modules map[string]int
	// baseOrder is the base relations in registration order, which is the Source's own order: a
	// did-you-mean tie goes to the earlier one, so the Source keeps control of which name wins.
	baseOrder []string
	// units is every AddModule call, in order. A unit is the scope of its private members.
	units []*unit
	// check caches the semantic pass over the units (see Check). Any registration resets it.
	check *checkState
}

// checkState is the cached result of Check: every unit's rules with their names resolved to paths,
// or the error that stopped resolution. Behind a pointer so the lazily-filled cache is shared rather
// than copied, and guarded because concurrent Evals may be the first to ask for it.
type checkState struct {
	mu       sync.Mutex
	done     bool
	err      error
	resolved [][]Rule // by unit index
}

// memberKind is what defines a path.
type memberKind int

const (
	kindBase memberKind = iota
	kindPredicate
	kindDerived
)

func (k memberKind) String() string {
	switch k {
	case kindPredicate:
		return "predicate"
	case kindDerived:
		return "derived relation"
	}
	return "base relation"
}

type member struct {
	kind   memberKind
	schema Schema  // kindBase
	pred   Builtin // kindPredicate
	unit   int     // kindDerived: index into Registry.units
}

// NewRegistry returns a registry holding every relation src serves, each as a base relation at the
// path it is named by. The Source's relations and schemas are read once, here: a relation the Source
// starts serving later is not in the tree until AddRelation registers it.
//
// It fails when two of the Source's own names break the tree's rules, such as `pin` beside
// `pin.net`, because a query could not name both. A nil src gives an empty registry.
func NewRegistry(src Source) (*Registry, error) {
	r := &Registry{src: src, members: map[string]member{}, modules: map[string]int{}, check: &checkState{}}
	if src == nil {
		return r, nil
	}
	for _, rel := range src.Relations() {
		s, ok := src.Schema(rel)
		if !ok {
			return nil, fmt.Errorf("query: source lists relation %q but serves no schema for it", rel)
		}
		if err := r.AddRelation(rel, s); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// MustRegistry is NewRegistry for a Source known to be well formed, such as a test fixture. It
// panics on the error NewRegistry would return.
func MustRegistry(src Source) *Registry {
	r, err := NewRegistry(src)
	if err != nil {
		panic(err)
	}
	return r
}

// AddRelation registers a base relation at path with the given schema. Its tuples come from the
// registry's Source, asked for by the same path, so a relation registered here must be one the
// Source can serve.
func (r *Registry) AddRelation(path string, s Schema) error {
	if err := r.add(path, member{kind: kindBase, schema: s}); err != nil {
		return err
	}
	r.baseOrder = append(r.baseOrder, path)
	return nil
}

// AddPredicate registers a computed predicate at path: a filter (Holds set) or a generator (Gen
// set), exactly one. It fails on a builtin that is neither or both, on a filter with arity below
// one, and on a path the tree's rules refuse (see Registry).
func (r *Registry) AddPredicate(path string, b Builtin) error {
	if (b.Holds == nil) == (b.Gen == nil) {
		return fmt.Errorf("query: predicate %q needs exactly one of Holds and Gen", path)
	}
	if b.Holds != nil && b.Arity < 1 {
		return fmt.Errorf("query: filter %q needs arity >= 1", path)
	}
	return r.add(path, member{kind: kindPredicate, pred: b})
}

// add places m at path after checking the path's shape and the tree's two rules.
func (r *Registry) add(path string, m member) error {
	if err := r.admits(path, m); err != nil {
		return err
	}
	r.put(path, m)
	return nil
}

// admits reports why m cannot be placed at path, or nil when it can.
func (r *Registry) admits(path string, m member) error {
	if err := checkPath(path); err != nil {
		return err
	}
	if prev, ok := r.members[path]; ok {
		return fmt.Errorf("query: %q is defined twice, as a %s and as a %s", path, prev.kind, m.kind)
	}
	if n := r.modules[path]; n > 0 {
		return fmt.Errorf("query: %q cannot be a %s: it is already a module holding %s", path, m.kind, strings.Join(r.children(path), ", "))
	}
	segs := strings.Split(path, ".")
	for i := 1; i < len(segs); i++ {
		mod := strings.Join(segs[:i], ".")
		if prev, ok := r.members[mod]; ok {
			return fmt.Errorf("query: %q needs %q to be a module, but it is a %s", path, mod, prev.kind)
		}
	}
	return nil
}

// put places m at path, which admits has accepted, and invalidates the semantic check.
func (r *Registry) put(path string, m member) {
	r.members[path] = m
	segs := strings.Split(path, ".")
	for i := 1; i < len(segs); i++ {
		r.modules[strings.Join(segs[:i], ".")]++
	}
	r.check = &checkState{}
}

// checkPath refuses a path with an empty segment or a character a query cannot spell, since a member
// registered there could never be called.
func checkPath(path string) error {
	if path == "" {
		return fmt.Errorf("query: empty path")
	}
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return fmt.Errorf("query: path %q has an empty segment", path)
		}
	}
	if !isRelation(path) {
		return fmt.Errorf("query: path %q holds a character a query cannot spell", path)
	}
	return nil
}

// Clone returns an independent copy sharing the Source, so a caller can register more without
// changing the registry it copied. A test adding a predicate uses it to leave a shared registry as it
// found it.
func (r *Registry) Clone() *Registry {
	out := &Registry{src: r.src, members: make(map[string]member, len(r.members)), modules: make(map[string]int, len(r.modules)), check: &checkState{}}
	for k, v := range r.members {
		out.members[k] = v
	}
	for k, v := range r.modules {
		out.modules[k] = v
	}
	out.baseOrder = append([]string(nil), r.baseOrder...)
	out.units = append([]*unit(nil), r.units...) // a unit is never changed after AddModule
	return out
}

// Source returns the Source the registry's base relations are read from.
func (r *Registry) Source() Source {
	if r == nil {
		return nil
	}
	return r.src
}

// schema returns the schema of a base relation at path.
func (r *Registry) schema(path string) (Schema, bool) {
	if r == nil {
		return Schema{}, false
	}
	m, ok := r.members[path]
	if !ok || m.kind != kindBase {
		return Schema{}, false
	}
	return m.schema, true
}

// predicate returns the builtin registered at path.
func (r *Registry) predicate(path string) (Builtin, bool) {
	if r == nil {
		return Builtin{}, false
	}
	m, ok := r.members[path]
	if !ok || m.kind != kindPredicate {
		return Builtin{}, false
	}
	return m.pred, true
}

// isModule reports whether anything is registered beneath path.
func (r *Registry) isModule(path string) bool { return r != nil && r.modules[path] > 0 }

// hasBase reports whether any base relation is registered. A registry with none cannot call a name
// unknown, because it cannot tell a typo from a vocabulary nobody installed yet (see Validate).
func (r *Registry) hasBase() bool { return r != nil && len(r.baseOrder) > 0 }

// children lists the direct children of a module, sorted, as their last segment.
func (r *Registry) children(module string) []string {
	seen := map[string]bool{}
	for p := range r.members {
		if leaf, ok := childOf(module, p); ok {
			seen[leaf] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// childOf reports the segment of path directly under module, if path lies beneath it.
func childOf(module, path string) (string, bool) {
	rest := path
	if module != "" {
		if !strings.HasPrefix(path, module+".") {
			return "", false
		}
		rest = path[len(module)+1:]
	}
	seg, _, _ := strings.Cut(rest, ".")
	return seg, true
}

// namesOf returns every path of the given kind, sorted.
func (r *Registry) namesOf(k memberKind) []string {
	if r == nil {
		return nil
	}
	var out []string
	for p, m := range r.members {
		if m.kind == k {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// derived returns the unit defining the derived relation at path.
func (r *Registry) derived(path string) (int, bool) {
	if r == nil {
		return 0, false
	}
	m, ok := r.members[path]
	if !ok || m.kind != kindDerived {
		return 0, false
	}
	return m.unit, true
}
