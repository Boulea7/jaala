package ns

import (
	"fmt"
	"sort"
)

// A Language is how a vocabulary learns what a module defines, without parsing anything itself. An
// engine provides one (jaala/datalog provides "datalog") and a host registers it with AddLanguage
// before adding modules written in it.
type Language interface {
	// Name is the tag modules are registered under, such as "datalog".
	Name() string
	// Members reports what a module's text defines: each PUBLIC member's bare name, arity, doc and
	// definition. A member the language keeps private is not reported and never gets a path. An error
	// is a module that does not parse or that the language refuses on its own terms.
	Members(text string) ([]MemberDecl, error)
	// Check validates every module of this language against the whole vocabulary, which may hold
	// modules it reads and modules that read it in any registration order, and returns the full
	// signature of each member those modules define, keyed by path. Vocabulary.Check calls it once
	// per state of the vocabulary.
	Check(v *Vocabulary) (map[string][]ArgSig, error)
}

// A MemberDecl is one public member a module defines, as its Language reports it.
type MemberDecl struct {
	Name  string // bare: the member's path is the module path plus this
	Arity int
	// Doc is the member's description, for Lookup.
	Doc string
	// Definition is the member's definition in its language's own syntax, one entry per clause, for a
	// host showing a reader how a member is defined.
	Definition []string
}

// A Module is one AddModule call: text in a language, registered at a module path.
type Module struct {
	Path     string
	Language string
	Text     string
	// Origin says where the text came from, such as the file it was read from, so a host can tell a
	// reader where a member is defined or which file to fix. It is whatever the host passed, and may
	// be empty.
	Origin  string
	Members []MemberDecl
}

// A ModuleError is a failure that one module is responsible for: a module AddModule refused, or a rule
// of a registered module that Check refused. Several modules may share a path, so Module and Origin
// are what tell a host which one, through errors.As; Error is the cause's message unchanged.
type ModuleError struct {
	// Module is the module's index into Modules(), or, for a module AddModule refused, the index it
	// would have had.
	Module int
	Path   string
	Origin string
	Err    error
}

// Error returns the cause's message, exactly as it would read without the module attached.
func (e *ModuleError) Error() string { return e.Err.Error() }

// Unwrap returns the cause.
func (e *ModuleError) Unwrap() error { return e.Err }

// AddLanguage makes a language available to AddModule. Registering a second language under one
// name is refused.
func (v *Vocabulary) AddLanguage(l Language) error {
	if _, ok := v.langs[l.Name()]; ok {
		return fmt.Errorf("query: language %q is already registered", l.Name())
	}
	v.langs[l.Name()] = l
	v.cache = newMemo()
	return nil
}

// AddModule registers text written in lang at module path ("" for the root), recording origin as
// where it came from (see Module.Origin; "" for nothing). The language reports the public members the
// text defines, and each is entered at path.name under the tree's rules, exactly as a relation or
// predicate would be: a member colliding with another path, or breaking module-or-member, is refused
// here, and nothing is registered when anything is refused. Every refusal is a *ModuleError.
//
// A language nobody registered is refused with its name. What a module's rules READ is not checked
// here, since what they read may be registered later; Check does that.
//
// Several modules may register at one module path, provided each member path has one definer, which
// is how a host's standard library and a project's own definitions share a module.
func (v *Vocabulary) AddModule(path, lang, text, origin string) error {
	id := len(v.mods)
	refuse := func(err error) error { return &ModuleError{Module: id, Path: path, Origin: origin, Err: err} }
	if path != "" {
		if err := checkPath(path); err != nil {
			return refuse(err)
		}
	}
	l, ok := v.langs[lang]
	if !ok {
		return refuse(fmt.Errorf("query: module %q is written in %q, and no language of that name is registered", path, lang))
	}
	decls, err := l.Members(text)
	if err != nil {
		return refuse(fmt.Errorf("%w (in module %q)", err, path))
	}
	for _, d := range decls {
		if err := v.admits(joinPath(path, d.Name), member{kind: kindDerived, module: id}); err != nil {
			return refuse(err)
		}
	}
	for _, d := range decls {
		v.put(joinPath(path, d.Name), member{kind: kindDerived, module: id})
	}
	v.mods = append(v.mods, Module{Path: path, Language: lang, Text: text, Origin: origin, Members: decls})
	v.cache = newMemo()
	return nil
}

// Modules returns every registered module, in registration order. DefiningModule indexes into it.
func (v *Vocabulary) Modules() []Module {
	if v == nil {
		return nil
	}
	return append([]Module(nil), v.mods...)
}

type checkKey struct{}

// Check validates every module, by asking each language that has modules to check them, and keeps
// the members' signatures for Lookup and Signature. It runs once per state of the vocabulary and is
// shared by every engine and query over it; a host calls it at load to report a broken library
// before any query runs.
func (v *Vocabulary) Check() error {
	_, err := v.signatures()
	return err
}

func (v *Vocabulary) signatures() (map[string][]ArgSig, error) {
	out, err := v.Memo(checkKey{}, func() (any, error) {
		used := map[string]bool{}
		for _, m := range v.mods {
			used[m.Language] = true
		}
		names := make([]string, 0, len(used))
		for n := range used {
			names = append(names, n)
		}
		sort.Strings(names)
		sigs := map[string][]ArgSig{}
		for _, n := range names {
			got, err := v.langs[n].Check(v)
			if err != nil {
				return nil, err
			}
			for p, s := range got {
				sigs[p] = s
			}
		}
		return sigs, nil
	})
	if err != nil {
		return nil, err
	}
	return out.(map[string][]ArgSig), nil
}

// Signature returns the full signature of the derived relation at path, declared or inferred, as its
// language's Check worked it out. It runs Check when it has not run, and reports false when the path
// is not a derived relation or the modules do not check.
func (v *Vocabulary) Signature(path string) ([]ArgSig, bool) {
	if _, ok := v.DefiningModule(path); !ok {
		return nil, false
	}
	sigs, err := v.signatures()
	if err != nil {
		return nil, false
	}
	s, ok := sigs[path]
	return s, ok
}

func joinPath(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}
