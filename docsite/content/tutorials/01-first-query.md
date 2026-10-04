---
title: "A first query"
description: "Facts, a goal, variables and constants, and the order jaala answers in."
next: {url: "/jaala/tutorials/02-rules-and-recursion/", title: "Rules and recursion"}
---

A query asks which values make a pattern true over the facts. Our service's [facts]({{ datasetURL "deps" }}) include `imports("api", "auth")`, which says the `api` package imports `auth`. To ask what `api` imports, we write the same shape with a variable where the answer goes:

{{ demo "demos/tutorials/01-imports-of-api.yaml" }}

`?p` is a variable, and `"api"` is a constant, which is pretty much the whole vocabulary of a goal. Everything before `=>` is the goal, the pattern to match, and everything after it is the projection, the columns the answer should have. Turn the question around by moving the constant, and you get every package that imports `log`:

{{ demo "demos/tutorials/01-importers-of-log.yaml" }}

When you don't care about a position, write `_`. It matches anything and binds nothing, so this asks for every package that imports something at all. `util` is missing because it imports nothing. Each package shows up once, even though most import several things, since an answer is a set of rows.

{{ demo "demos/tutorials/01-wildcard.yaml" }}

## Joining

A goal can have several parts, separated by commas, and a variable that appears in two of them has to take the same value in both. This finds the platform team's packages and their sizes:

{{ demo "demos/tutorials/01-join.yaml" }}

## The order of the answer

jaala sorts every answer the same way, column by column, so the same question over the same facts always comes back in the same order. Numbers sort by value, not as text, which is why `1200` comes after `900` here rather than before `150`:

{{ demo "demos/tutorials/01-order.yaml" }}

In a column holding both, numbers come before text. `order by` and `limit` choose a different order when you want one, and the [guide]({{.Site.PathPrefix}}/guide/ordering/) covers them.

## Where an answer came from

Every row carries citations, the facts that produced it. Here each row cites the one `imports` fact it matched. Once rules get involved, a row cites everything that went into it, which the next tutorial shows.

{{ demo "demos/tutorials/01-cites.yaml" }}

## When a query is wrong

jaala checks a query against what it knows before running it, and refuses one that can't be right rather than answering nothing without saying why. `imports` takes two arguments, so asking it with one is an error, and the message says so:

{{ demo "demos/tutorials/01-arity.yaml" }}

Every error starts with `query:`, so a host can tell a mistake in the question from a failure somewhere else.
