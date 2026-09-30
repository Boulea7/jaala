# jaala

Jaala (Sanskrit for "network") is a small query engine for graph-shaped data.

Its first package, `datalog`, is a Datalog evaluator with stratified negation, recursion,
aggregation and `having`, binding-pattern indexes, and provenance: every answer row carries the
citations of the facts that produced it. It knows nothing about any particular domain. A host
registers its names in a `Registry`, one tree of paths such as `edge`, `str.contains` or
`acme.power.rail_budget`: base relations whose facts come from a `Source`, and computed predicates.

It started as the query engine inside [agni](https://github.com/panyam/agni), an EDA tooling
engine, and was extracted so other graph tools could share it.

The package imports only the Go standard library and builds for `GOOS=js GOARCH=wasm`.

Licensed under Apache-2.0.
