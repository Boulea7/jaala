---
title: "Negation"
description: "Asking what isn't there with not, and the two rules that keep it well defined: anchoring and stratification."
prev: {url: "/jaala/tutorials/02-rules-and-recursion/", title: "Rules and recursion"}
next: {url: "/jaala/tutorials/04-aggregation/", title: "Aggregation"}
---

`not` asks for something to be absent. A package is untested when no `test` fact names it, so:

{{ demo "demos/tutorials/03-untested.yaml" }}

For each package, `not test(_, ?p)` holds when there's no test of any name for that package. The `_` inside a `not` means "any value": we're asking that no test exists at all, not about some particular one. The same shape finds the packages nothing imports, which are where a dependency graph starts:

{{ demo "demos/tutorials/03-unimported.yaml" }}

## Negation in a rule

A rule can use `not` too, and other rules and goals can build on it. This defines `untested` once, then asks which of `app`'s dependencies are untested:

{{ demo "demos/tutorials/03-untested-deps.yaml" }}

## A `not` has to be anchored

A `not` is checked row by row, so it has to share a variable with the positive part of the goal, the part that produces rows. Here `?t` and `?q` appear only inside the `not`, so it doesn't depend on the row at all:

{{ demo "demos/tutorials/03-unanchored.yaml" }}

As written, that `not` would hold for every package or for none, depending on whether any test exists anywhere, which is almost never what the author meant. So jaala refuses it and says which variable has nothing to range over. The fix is usually a variable that was meant to be shared: `not test(?t, ?p)` asks about this package's tests.

## Stratification

A rule can't depend on its own negation. This one says a package is odd when it isn't odd, which has no consistent answer:

{{ demo "demos/tutorials/03-not-stratifiable.yaml" }}

jaala sorts rules into strata, so that everything a `not` reads is fully derived before the rule reading it runs. A cycle through a `not` (a rule reaching its own negation, here directly, often through other rules) can't be sorted, so the query is refused up front rather than answered from a half-built relation. Recursion without negation, like `depends_on`, is fine.
