package guide_test

import (
	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
	"github.com/panyam/jaala/stdlib"
)

// deps ships the dependency rules as a module. A comment above a member's
// first rule is its doc. _step is private to the module: its own rules can
// call it, and a query can't.
const deps = `
# Everything a package depends on, directly or through others.
depends_on(?a, ?b) :- _step(?a, ?b);
depends_on(?a, ?c) :- depends_on(?a, ?b), _step(?b, ?c);

_step(?a, ?b) :- imports(?a, ?b);
`

// source serves the imports, and a pkg relation whose license column is
// absent where the package states none, which isn't the same as stating "".
func source() *ns.MemSource {
	src := ns.NewMemSource().
		Declare("imports", "from", "to").
		Declare("pkg", "name", "license")
	edges := [][2]string{{"app", "api"}, {"api", "log"}, {"log", "util"}}
	for _, e := range edges {
		src.Add("imports", ns.Tuple{Vals: []ns.Value{ns.S(e[0]), ns.S(e[1])}})
	}
	src.Add("pkg", ns.Tuple{Vals: []ns.Value{ns.S("app"), ns.S("MIT")}})
	src.Add("pkg", ns.Tuple{Vals: []ns.Value{ns.S("api"), ns.Absent()}})
	src.Add("pkg", ns.Tuple{Vals: []ns.Value{ns.S("log"), ns.S("")}})
	src.Add("pkg", ns.Tuple{Vals: []ns.Value{ns.S("util"), ns.S("Apache-2.0")}})
	return src
}

// vocabulary names everything a query can call, with the deps module.
func vocabulary(src ns.Source) (*ns.Vocabulary, error) {
	v, err := ns.NewVocabulary(src)
	if err != nil {
		return nil, err
	}
	if err := v.AddLanguage(datalog.Language); err != nil {
		return nil, err
	}
	if err := stdlib.Register(v); err != nil {
		return nil, err
	}
	err = v.AddModule("deps", datalog.LanguageName, deps, "deps.dl")
	if err != nil {
		return nil, err
	}
	return v, v.Check()
}
