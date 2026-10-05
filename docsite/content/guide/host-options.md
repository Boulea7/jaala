---
title: "Host options"
description: "Bind, Budget and the context: how a Go host gives a query its values, caps its work, and stops it."
prev: {url: "/jaala/guide/ordering/", title: "Ordering"}
next: {url: "/jaala/guide/modules-and-vocabulary/", title: "Modules and the vocabulary"}
---

A host runs a query with `Eval`, and passes options to it. This page covers the three a host uses most. The [Go tutorial]({{.Site.PathPrefix}}/tutorials/05-go-host/) builds the vocabulary and base they run against, and shows the fourth, `Witnesses`. Every code block here is a Go `Example` from `docsite/examples/guide/`, which `go test` runs on every build of this site and checks against its `// Output:` comment.

The examples share a source and a vocabulary, built the same way as in the tutorial, with a `deps` module of rules:

```go
{{ includeFileText "examples/guide/vocabulary_test.go" }}
```

## `Bind`, `Budget` and the context

```go
{{ includeFileText "examples/guide/options_test.go" }}
```

`Bind` gives a goal variable its value from Go, so the query text stays fixed and nobody builds queries out of strings. A bound variable behaves as if its value were written into the goal, and that includes counting as the anchor of a `not` (see [negation]({{.Site.PathPrefix}}/guide/negation/#a-not-has-to-be-anchored)). The answer still has the bound column, filled with the value. Binding a variable the goal doesn't use is an error, since it's pretty much always a typo, and a typo there would otherwise answer the unbound question without a word.

A variable can be bound to several values too, which is how a host asks one question about a whole selection, such as every package a user ticked. The variable then ranges over the values, as if the goal were joined with a relation holding exactly them, so the answer is the union, and an aggregate like `count(distinct ?d)` counts across the whole set, which asking once per value can't do. Demand starts from every value, so a recursive relation is still worked out only for the values asked, and one query costs about what the separate ones would. Each value is checked as a single one is, and an error names the one refused. A variable bound to no values matches nothing: the answer is empty, or the one row an aggregate gives over nothing (a count of 0). A set of one value is the same as binding that value.

`Budget` caps the work one `Eval` does, counted the same way on every run, so a budget that passes once passes every time. `Base.Work` reports what a query used, which is how to size a budget from your real queries. The budget is what bounds memory too: a goal that aggregates holds its bindings until it reduces them, which can be a few kilobytes per unit of work ([#120](https://github.com/panyam/jaala/issues/120)), so size it with the memory you have in mind as well as the time.

The context reaches every `Eval`. When it's cancelled or its deadline passes, the `Eval` stops at its next check, which comes every 1024 units of work, and returns an error that wraps the context's own, so `errors.Is(err, context.DeadlineExceeded)` works. It also reaches a host's own Go code running inside a query, its generators and sources, so those can stop too.

## Sharing a base

A `Base` pairs a vocabulary with one source's facts, and caches and indexes what it reads. It's safe to share between `Eval`s running at the same time, since each `Eval` keeps its own state. Build one per dataset, and reuse it for every query over that dataset, which mostly comes free once the vocabulary is built at startup.

It also keeps the derived relations a query evaluated in full, so the next query calling the same relation reads it instead of deriving it again ([#140](https://github.com/panyam/jaala/issues/140)). A library member that three workbook tabs all call is derived once. The key is the relation's rules and everything they read, so a query that sends its own definition of a relation derives its own. A relation derived only for the values one query asked about isn't kept, since it's partial. Only `SemiNaive` uses the cache, and not under `Witnesses`. `Base.LimitDerivedCache` sets how many derived tuples the base keeps (about a million by default, and 0 turns it off), dropping the relations used least recently first.

All of this assumes the source's facts don't change while the base is kept. If yours can, have the source implement `ns.Versioned`, whose `Version` string changes whenever the facts do. Each `Eval` reads it first, and when it has moved, the base drops everything it cached and reads the source again. A host that changes facts in place without a version calls `Base.Forget` instead. A generator whose answers can change while the facts don't (one reading the clock, say) sets `Volatile` on its `ns.Builtin`, and no relation reading it is kept.
