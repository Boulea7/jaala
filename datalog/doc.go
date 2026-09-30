// Package datalog is a Datalog engine for graph-shaped data: a small declarative query language
// whose every answer carries the provenance of the facts that produced it.
//
// It supports conjunction, comparison, stratified negation, recursion to a fixpoint, aggregation
// with `having` and `distinct`, and computed predicates. A Query is the whole program: rules that
// define derived relations, a goal to solve, and the columns to answer with. Parse reads the text
// syntax; the builders (V, Rel, Pos, Def, Build, ...) construct the same Query in code.
//
// The engine knows nothing about any domain. A host supplies:
//
//   - a Source, serving its base relations as positional Tuples with citations;
//   - Predicates, the filters and generators a query may call. StandardPredicates covers the string
//     filters; a host adds its own, such as a walk over its graph.
//
// NewBase joins the two, and an Evaluator (Naive) answers a Query over the Base. A Base caches and
// indexes what it reads, so one Base serves many queries, concurrently.
//
// Evaluation is guaranteed to terminate because no rule can invent a value: every answer is built
// from constants in the facts and values a host's generators draw from finite data. That guarantee
// is why the value model is scalar for now.
//
// The engine was extracted from agni (github.com/panyam/agni), an EDA tool that queries circuit
// designs, and the examples in these comments use its vocabulary: component-on-net(ref, net) places
// a part on a net, reaches(from, to) walks a circuit's series connections, and so on. References to
// "agni issue N" point at the history behind a rule.
//
// The package imports only the standard library and builds for GOOS=js GOARCH=wasm.
package datalog
