// Package datalog is a Datalog engine for graph-shaped data: a small declarative query language
// whose every answer carries the provenance of the facts that produced it.
//
// It supports conjunction, comparison, stratified negation, recursion to a fixpoint, aggregation
// with `having` and `distinct`, and computed predicates. A Query is the whole program: rules that
// define derived relations, a goal to solve, and the columns to answer with. Parse reads the text
// syntax; the builders (V, Rel, Pos, Def, Build, ...) construct the same Query in code.
//
// The engine knows nothing about any domain. A host supplies a Registry: one namespace tree in which
// every name a query can call lives at a path, its segments separated by "." (edge, str.contains,
// acme.power.rail_budget). A leaf of the tree is one of:
//
//   - a base relation, whose tuples come from the host's Source;
//   - a predicate, a filter or generator the host computes. StandardPredicates registers the string
//     tests under str (str.contains, str.prefix, ...) and absent at the root; a host adds its own,
//     such as a walk over its graph;
//   - a derived relation, written as Datalog rules and registered with AddModule, so a relation
//     defined once sits beside the base relations it builds on (net.has_test_point next to
//     net.pin_count) and any query can call it.
//
// A query naming a derived relation pulls in the module that defines it, and whatever that module
// reads, when it is linked (Link, which Eval does first). A linked query is an ordinary query: the
// evaluator never sees a module.
//
// A segment is a module or a member, never both, and each path has one definer; the registry refuses
// anything else when it is registered. Inside a module's text a bare name is that module's member
// first and the root's second. Registry.Check resolves and validates every module together, so a
// broken library is reported before any query runs.
//
// Every member has a signature: per argument, a name and an ArgType saying what it denotes (an
// opaque entity kind such as "net", a kind taken per row from another argument, a kind located
// through an owner argument, or a scalar type with a unit), optionally closed over a vocabulary. Base
// relations and predicates declare theirs; a derived relation may declare its own in a rule head,
// `has_test_point(?n: net) :- ...`, and what it leaves undeclared is inferred through its rules and
// marked so. Registry.Lookup and Registry.Members answer what is at a path, for a host offering
// drill-down discovery, and ColumnKinds carries the kinds through to a query's answer columns.
//
// NewBase builds a fact base over a Registry, and an Evaluator (Naive) answers a Query over the
// Base. A Base caches and indexes what it reads, so one Base serves many queries, concurrently.
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
