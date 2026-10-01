# jaala

Jaala (Sanskrit for "network") is a small query engine for graph-shaped data.

Its first package, `datalog`, is a Datalog evaluator with stratified negation, recursion,
aggregation and `having`, binding-pattern indexes, and provenance: every answer row carries the
citations of the facts that produced it. It knows nothing about any particular domain.

It comes in two packages:

- `ns` is the vocabulary: one tree of dotted paths holding every name a query can call, with each
  name's signature. A leaf is a base relation whose facts some `Source` serves, a predicate the host
  computes, or a derived relation defined by a module. `ns` parses and evaluates nothing, and imports
  only the standard library, so a host's fact layer can register into it without taking on an engine.
- `datalog` is the engine. It provides Datalog as a module language, links the modules a query
  names, and evaluates queries over a vocabulary paired with one `Source`.

```go
v, _ := ns.NewVocabulary(nil)                     // names only; no data
v.AddRelation("component.net", ns.Schema{Arity: 2, Labels: []string{"ref_des", "net"}})
v.AddRelation("component.class", ns.Schema{Arity: 2, Labels: []string{"ref_des", "class"}})
ns.StandardPredicates(v)                          // str.contains, str.prefix, ..., absent
v.AddLanguage(datalog.Language)
v.AddModule("net", datalog.LanguageName, `
# Nets that carry a test point.
has_test_point(?n: net) :- component.net(?tp, ?n), component.class(?tp, "test_point");
`)
if err := v.Check(); err != nil { ... }           // once, at load

base, err := datalog.NewBase(v, designSource)     // per dataset
rows, err := datalog.SemiNaive{}.Eval(datalog.MustParse(`net.has_test_point(?n) => ?n`), base)
```

A query naming `net.has_test_point` pulls in the module that defines it. Each member carries a
signature (argument names, entity kinds, scalar types and units), declared or inferred through its
rules, which `v.Lookup(path)` returns for a host that lists and drills into what is available.

It started as the query engine inside [agni](https://github.com/panyam/agni), an EDA tooling
engine, and was extracted so other graph tools could share it.

Both packages import only the Go standard library (and `datalog` imports `ns`), and build for
`GOOS=js GOARCH=wasm`.

Licensed under Apache-2.0.
