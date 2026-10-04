---
title: "Tutorials"
description: "Learn jaala's query language by asking questions about a small Go service: what it imports, what's tested, who owns what."
---

These tutorials teach jaala's query language one idea at a time, all against the same small fact set: a Go-style service with ten packages, what each one imports, its tests, which team owns each package, and how many lines each has. Every example on these pages runs on jaala when the site is built, so the answers you see are the engine's.

1. [A first query](01-first-query/) covers facts, goals and variables, and the order answers come back in.
2. [Rules and recursion](02-rules-and-recursion/) derives new relations and follows imports all the way down.
3. [Negation](03-negation/) asks what isn't there, and explains the two rules that keep that well defined.
4. [Aggregation](04-aggregation/) counts, sums and lists, in the answer and in a rule.

A fifth tutorial, on embedding jaala in a Go program, is on its way.

## The facts

Here's the whole fact set. A fact is a relation name with its arguments, and each line holds a few of them, separated by `;`.

```text
{{ includeFileText "demo/fixtures/deps.facts" }}
```

`cache` and `config` import each other on purpose, because the second tutorial needs a cycle.
