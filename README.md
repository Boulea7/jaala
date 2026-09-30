# jaala

Jaala (Sanskrit for "network") is a small query engine for graph-shaped data.

Its first package, `datalog`, is a Datalog evaluator with stratified negation, recursion,
aggregation and `having`, binding-pattern indexes, and provenance: every answer row carries the
citations of the facts that produced it. It knows nothing about any particular domain. A host
supplies its facts through a `Source` and its computed predicates through `Predicates`.

It started as the query engine inside [agni](https://github.com/panyam/agni), an EDA tooling
engine, and was extracted so other graph tools could share it.

The package imports only the Go standard library and builds for `GOOS=js GOARCH=wasm`.

Licensed under Apache-2.0.
