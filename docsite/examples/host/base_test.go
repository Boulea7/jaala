package host_test

import (
	"github.com/panyam/jaala/datalog"
	"github.com/panyam/jaala/ns"
	"github.com/panyam/jaala/stdlib"
)

// deps is a module, the rules a host ships as a library. A query calls them
// by path, as deps.depends_on.
const deps = `
depends_on(?a, ?b) :- imports(?a, ?b);
depends_on(?a, ?c) :- depends_on(?a, ?b), imports(?b, ?c);
`

// base builds what a query runs against. The vocabulary names everything a
// query can call, which is the Source's relations, the standard library and
// the modules. A host builds and checks it once, at startup, and builds a
// Base per dataset. A Base is safe to share across concurrent queries.
func base(src ns.Source) (*datalog.Base, error) {
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
	if err := v.Check(); err != nil {
		return nil, err
	}
	return datalog.NewBase(v, src)
}
