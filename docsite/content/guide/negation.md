---
title: "Negation"
description: "What a not means, which variables can appear inside it, how a value the host binds anchors it, and why some programs are refused."
prev: {url: "/jaala/guide/types-and-constants/", title: "Types and constants"}
next: {url: "/jaala/guide/aggregation/", title: "Aggregation"}
---

`not` followed by an atom holds when no tuple matches the atom. jaala checks it for each row the rest of the goal produces, after that row's variables have their values, which is negation as failure: absent from the facts and the rules means false.

## Variables inside a `not`

A variable inside a `not` is either bound by the rest of the goal or appears only inside the `not`. One that appears only there means "any value", the same as `_`. So this goal asks that no test of any name covers the package, exactly as `not test(_, ?p)` would:

{{ demo "demos/guide/negation-existential.yaml" }}

The answer can't have a column for a variable like `?t`, since a `not` that holds has found no value for it. jaala refuses a projection that names one.

## A `not` has to be anchored

At least one variable of the `not` has to be bound by the rest of the goal or the rule, so that the `not` asks something different for each row. With nothing anchoring it, a `not` would hold for every row or for none:

{{ demo "demos/guide/negation-unbound.yaml" }}

A value the host binds with `Bind` counts as an anchor, the same as one the goal binds itself. That's the usual case for a host asking about one thing it already has in hand. Bound to `cache`, which no test covers, the same goal answers one row:

{{ demo "demos/guide/negation-host-bound.yaml" }}

Bound to `auth`, which has tests, it answers none:

{{ demo "demos/guide/negation-host-bound-tested.yaml" }}

jaala checks anchoring against the query as you wrote it, before it binds the host's values or rewrites anything, so an error always names a variable you wrote.

## Stratification

Before it runs a `not`, jaala derives everything the `not` reads. It sorts the rules into strata so that's always possible: a relation read under `not` sits in a lower stratum than the rule reading it, and so does the body of an [aggregate in a rule's head]({{.Site.PathPrefix}}/guide/aggregation/#an-aggregate-in-a-rule). This lets one rule build on another's negation:

{{ demo "demos/guide/negation-strata.yaml" }}

A cycle that passes through a `not` can't be sorted that way. Here `a` holds where `b` doesn't, and `b` holds where `a` doesn't, so there's no order to derive them in and no single answer:

{{ demo "demos/guide/negation-mutual.yaml" }}

jaala refuses such a program before it runs any rule, and names the relations in the cycle. Recursion with no `not` in the cycle, like the `depends_on` rules in the tutorials, is fine.
