# jaala

Jaala (Sanskrit for "network") is a small query engine for graph-shaped data.

Its first package, `datalog`, is a Datalog evaluator with stratified negation, recursion,
aggregation and `having`, binding-pattern indexes, and provenance: every answer row carries the
citations of the facts that produced it. It knows nothing about any particular domain.

A host registers every name a query can call in a `Registry`, one tree of dotted paths. A leaf is a
base relation whose facts come from a `Source`, a predicate the host computes, or a derived relation
written in Datalog and registered as a module:

```go
reg, _ := datalog.NewRegistry(src)          // net.pin_count, component.net, ... from the Source
datalog.StandardPredicates(reg)             // str.contains, str.prefix, ..., absent
reg.AddModule("net", `
# Nets that carry a test point.
has_test_point(?n: net) :- component.net(?tp, ?n), component.class(?tp, "test_point");
`)
rows, err := datalog.Naive{}.Eval(datalog.MustParse(`net.has_test_point(?n) => ?n`), datalog.NewBase(reg))
```

A query naming `net.has_test_point` pulls in the module that defines it. Each member carries a
signature (argument names, entity kinds, scalar types and units), declared or inferred through its
rules, which `reg.Lookup(path)` returns for a host that lists and drills into what is available.

It started as the query engine inside [agni](https://github.com/panyam/agni), an EDA tooling
engine, and was extracted so other graph tools could share it.

The package imports only the Go standard library and builds for `GOOS=js GOARCH=wasm`.

Licensed under Apache-2.0.
