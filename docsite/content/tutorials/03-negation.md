---
title: "Negation"
description: "Asking what isn't there with not, and the two rules that keep it well defined: anchoring and stratification."
prev: {url: "/jaala/tutorials/02-rules-and-recursion/", title: "Rules and recursion"}
next: {url: "/jaala/tutorials/04-aggregation/", title: "Aggregation"}
---

`not` asks for something to be absent. A package is untested when no `test` fact names it, so:

{{ demo "demos/tutorials/03-untested.yaml" }}

For each package, `not test(_, ?p)` holds when there's no test of any name for that package. A `_` inside a `not` stands for any value, so you're asking that no test exists at all rather than about one particular test. The same shape finds the packages nothing imports, which are where a dependency graph starts:

{{ demo "demos/tutorials/03-unimported.yaml" }}

## Negation in a rule

A rule can use `not` too, and other rules and goals can build on it. This defines `untested` once, then asks which of `app`'s dependencies are untested:

{{ demo "demos/tutorials/03-untested-deps.yaml" }}

## A `not` has to be anchored

jaala checks a `not` row by row, so it has to share a variable with the positive part of the goal, the part that produces rows. Here `?t` and `?q` appear only inside the `not`, so it doesn't depend on the row at all:

{{ demo "demos/tutorials/03-unanchored.yaml" }}

As written, that `not` would hold for every package or for none, depending on whether any test exists anywhere, which is almost never what you meant. So jaala refuses it and says which variable has nothing to range over. The fix is usually a variable that was meant to be shared, and `not test(?t, ?p)` asks about this package's tests.

## Stratification

A rule can't depend on its own negation. This one says a package is odd when it isn't odd, which has no consistent answer:

{{ demo "demos/tutorials/03-not-stratifiable.yaml" }}

jaala sorts rules into strata, so that it finishes deriving everything a `not` reads before it runs the rule reading it. A cycle through a `not` (a rule reaching its own negation, here directly, more often through other rules) can't be sorted, so jaala refuses the query up front rather than answer from a half-built relation. Recursion without negation, like `depends_on`, is fine, and it's a bit of a relief that the line falls there.
