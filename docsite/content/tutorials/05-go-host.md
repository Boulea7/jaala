---
title: "Embedding jaala in Go"
description: "Serve facts from Go, ship rules as a module, run a query with Bind and Budget, and read its citations, witnesses and errors."
prev: {url: "/jaala/tutorials/04-aggregation/", title: "Aggregation"}
---

The first four tutorials wrote queries against facts the site provided. A real host provides the facts itself, from whatever it already has: a parsed `go.mod`, a netlist, a database. This tutorial builds the pieces you need as a host, in Go, over a few of the same imports.

Every code block on this page is a file from `docsite/examples/host/`, included as is. The last three are Go `Example` functions, so `go test` runs them on every build of this site and checks their output against the `// Output:` comment at the end. If jaala's API or answers change, the build fails before the page can drift.

## Facts from Go

A `Source` serves a relation's tuples. `ns.MemSource` is the simplest one, a set of tuples held in memory, which is pretty much all a test or a small host needs. Each tuple carries its citations, which is how an answer can point back at the line of a file it came from:

```go
{{ includeFileText "examples/host/source_test.go" }}
```

## A vocabulary and a base

The vocabulary is everything a query can name: the source's relations, the standard library (`str.contains` and friends), and modules of rules. Here the rules from the second tutorial become a module called `deps`, so a query calls `deps.depends_on`. `Check` resolves the modules and reports a mistake in one, with the file it came from, before any query runs.

```go
{{ includeFileText "examples/host/base_test.go" }}
```

A host usually builds the vocabulary once at startup and a `Base` per dataset. A `Base` caches what it reads, and it's safe to share between queries running at the same time.

## Running a query

`MustParse` turns query text into a `Query`, and `SemiNaive.Eval` answers it. `Bind` fills a goal variable from Go, so one query serves every package and you never build query text by hand. `Budget` caps the work, so a query over a big graph stops instead of running on:

```go
{{ includeFileText "examples/host/query_test.go" }}
```

Each row's `Bind` maps a column to its value, and `Cites` is the sorted set of citations behind it. `api` depends on `util` through `log`, so that row cites both imports.

## Witnesses

Citations say which facts an answer used. Ask for `Witnesses` and each row also says how they were combined, as a tree of the rules that fired:

```go
{{ includeFileText "examples/host/witness_test.go" }}
```

The tree follows the program as written, even though the evaluator may have rewritten it quite a bit to run faster, so you can show it to the person who wrote the rules.

## Errors

A query that can't be right fails with an error whose text starts with `query:`, which is safe to show whoever wrote the query. Running out of budget is its own type, so a host can tell "this question is too big" apart from "this question is wrong":

```go
{{ includeFileText "examples/host/errors_test.go" }}
```

A host runs this loop for as long as it lives. Most of the work after this page is deciding which facts to serve and which rules belong in a module.
