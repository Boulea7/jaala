---
title: "Evaluation"
description: "How a query becomes an answer: linking, checking the program as written, strata, and a fixpoint run component by component."
prev: {url: "/jaala/architecture/layering/", title: "Layering"}
next: {url: "/jaala/architecture/rewrites/", title: "Rewrites"}
---

Every evaluator runs a query through the same steps, and they differ only in the last one.

1. **Parse** the text into rules and a goal.
2. **Bind** the host's values into the goal (`Bind`), keeping the goal as written for the checks that need it. A variable bound to one value becomes a constant. One bound to several stays a variable, and after the checks in step 5 the goal is joined with a relation holding its values, which the demand rewrite starts from.
3. **Link** the names. A name a query calls from a module pulls in that module's rules, and whatever those read, so a query only carries the rules it can reach.
4. **Coerce** constants to their arguments' types, so `"800"` in a number argument is the number 800 (see [types]({{.Site.PathPrefix}}/guide/types-and-constants/)).
5. **Check** the program as written: every call's arity and every goal name, generators' modes, every negation's anchor, and every rule (known relations, consistent arity, bound head variables, stratification). Everything is checked before any rewrite renames it, so an error names what you wrote.
6. **Rewrite**, for the planned `SemiNaive` only (see [rewrites]({{.Site.PathPrefix}}/architecture/rewrites/)).
7. **Derive** the rules to a fixpoint, then solve the goal over the result, then aggregate, order and limit.

## The base and each evaluation

A `Base` pairs a vocabulary with one source's facts. It caches and indexes what it reads from the source, and the derived relations `SemiNaive` evaluated in full, keyed by their rules ([#140](https://github.com/panyam/jaala/issues/140)), and it's shared: many evaluations run against one `Base` at once. Everything an evaluation changes lives on its own copy of the base: the relations its rules derive, its budget and its context. What the copies share is either read-only, counted atomically, or locked. jaala's tests run concurrent evaluations under the race detector to keep that true.

## Strata

Rules that read a relation under `not`, or reduce it in an aggregate, can only run once that relation is complete. So jaala sorts relations into strata: a relation read positively can share a level with its reader, and one read under `not` or by an aggregate has to sit lower. A cycle that needs a relation lower than itself can't be sorted, and the query is refused (see [negation]({{.Site.PathPrefix}}/guide/negation/#stratification)).

## The fixpoint

`Naive` evaluates every rule of a stratum, in written order, again and again until a round derives nothing new. It's simple enough to trust, which is pretty much the whole reason we keep it as the reference.

`SemiNaive` splits each stratum into strongly connected components (Tarjan's algorithm) and derives them in dependency order. A relation that doesn't read itself, even through others, is derived once. Inside a recursive component, each round runs a variant of each recursive rule in which one recursive literal reads only the previous round's new tuples, the delta. Each tuple is then joined against once, rather than once per round. When the delta is smaller than the literal the plan would start from, the round starts from the delta.

Both count their work the same way on every run, through one counter that also checks the budget, and the context every 1024 units. The [evaluators]({{.Site.PathPrefix}}/reference/evaluators/) reference shows the three ways to run them answering alike:

```go
{{ includeFileText "examples/reference/evaluators_test.go" }}
```
