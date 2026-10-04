---
title: "What jaala is"
description: "A Datalog engine you embed in a Go program, which knows nothing about your domain and cites the facts behind every answer."
---

jaala is a Datalog engine for graph-shaped data. A host program, written in Go, gives it facts (parts on nets, edges in a graph, tests and the code they cover) and asks questions as Datalog rules and a goal. jaala works out the answer and names the facts behind every row. It started as the query engine inside [agni](https://github.com/panyam/agni), an EDA tool, and we pulled it out so other graph tools could share it. [Declaire](https://github.com/panyam/declaire) is the second host.

## A query

Here's a reachability question over a directed graph. The two rules define `reach`, and the line after `=>` is the goal: everything `a` reaches.

```datalog
reach(?a, ?b) :- edge(?a, ?b);
reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c);
reach("a", ?x) => ?x
```

Over the edges a→b, b→c and c→d, the answer is `b`, `c` and `d`, and the row for `d` cites the three edges on the way there.

## What the language has

- Rules that define derived relations, and recursion through them to a fixpoint.
- Stratified negation, so `not covered(?n)` reads a relation that's already complete.
- Aggregation in the answer (`count`, `sum`, `min`, `max`, `list`, with `distinct` and `having`) and in a rule's head.
- `order by`, `limit` and `offset`, over an answer that's sorted the same way every time.
- Types on a relation's arguments, declared or inferred, so a constant of the wrong type is refused rather than matching nothing without an error.

## Provenance

Every answer row carries the citations of the facts that produced it, so a host can point at the evidence: the net, the part, the line in the source file. Asked for witnesses, jaala also returns the tree of rules and facts that derived the row.

## Three packages

jaala comes as three packages, layered one way.

- `ns` is the vocabulary: the tree of names a query can call, each with its signature. A name is a base relation whose facts a host's `Source` serves, a predicate the host computes, or a derived relation a module defines. It doesn't parse or evaluate anything, which is the engine's job.
- `stdlib` is the standard vocabulary: string tests like `str.contains` and `str.glob`, and `absent`.
- `datalog` is the engine. It parses queries, links the modules a query names, and evaluates it over a vocabulary paired with a `Source`.

All three use only the Go standard library, and they build for `GOOS=js GOARCH=wasm`, which is how the examples on this site will run in your browser.

## Two evaluators

`Naive` evaluates rules in the order they're written and repeats every rule until nothing changes. It's slow on purpose, and fairly easy to convince yourself is right, which is why every faster strategy is checked against it. `SemiNaive` is the one to use. It only revisits what changed in the last round, derives just what a query's constants ask for, and plans each rule body, so the order a rule is written in doesn't change what it costs.

This site documents jaala at the commit shown in the footer, which is usually a little ahead of the latest release.
