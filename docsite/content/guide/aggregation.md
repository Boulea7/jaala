---
title: "Aggregation"
description: "Groups, counting rows against counting values, having, an aggregate in a rule's head, and what an aggregate answers over nothing."
prev: {url: "/jaala/guide/negation/", title: "Negation"}
next: {url: "/jaala/guide/ordering/", title: "Ordering"}
---

An aggregate in the projection turns the goal's rows into one row per group. The plain variables in the projection make up the group's key, and each aggregate reduces the rows that share a key. The aggregates are `count`, `sum`, `min`, `max` and `list`, and each takes one variable.

## Groups

A key can have several columns. Here each team and package is a group, and the count is of the tests in it:

{{ demo "demos/guide/aggregation-two-keys.yaml" }}

A group exists only when some row has its key, so a package with no tests has no row here at all. An aggregate in the body counts it as zero instead ([below](#an-aggregate-in-a-body)).

## Counting rows or values

`count(?t)` counts the group's rows, the bindings of the goal. `count(distinct ?t)` counts the different values `?t` takes. They differ whenever a value repeats, which a join makes pretty common. `api` imports four packages, owned by three teams:

{{ demo "demos/guide/aggregation-bindings-vs-values.yaml" }}

`list` gathers a group's values, sorted the way answers are, and `list(distinct ?x)` drops the repeats.

## Numbers only

`sum`, `min` and `max` reduce the group's numbers. Over numbers they do what you'd expect:

{{ demo "demos/guide/aggregation-min-max.yaml" }}

A group with no numbers in that column has no least or greatest, so `min` and `max` answer `absent`, and `sum` answers 0, as it would over no rows at all (below). That happens over a text column, like package names ([#122](https://github.com/panyam/jaala/issues/122)). `absent` sorts first and is never equal to the empty string, and a comparison like `?m < 5` or `having min(?p) < 5` is false for it, so it can't pass for a number:

{{ demo "demos/guide/aggregation-min-text.yaml" }}

## Filtering groups

`having` filters groups by what an aggregate came to, after the reduce. The aggregate doesn't have to be in the projection, which is how you ask for the subjects rather than the tally. These are the packages at least four others import:

{{ demo "demos/guide/aggregation-having-unselected.yaml" }}

A comparison in the goal can't do this, since it runs on each row before there's a group to count.

## An aggregate over nothing

With no plain variables in the projection, the whole answer is a single group, and that group exists even when no row matched. So an aggregate over nothing still answers one row, with `count` and `sum` at zero and `min` and `max` absent:

{{ demo "demos/guide/aggregation-empty.yaml" }}

## An aggregate in a body

`?n = count(?t) : { test(?t, ?p) }` binds `?n` to an aggregate over the bindings of its own body, the part in braces. The braces are reduced once per value of the variables they share with the rest of the clause, here `?p`, and every value the rest of the clause gives `?p` gets one, so a package with no tests counts 0 rather than having no row:

{{ demo "demos/guide/aggregation-body-count.yaml" }}

That makes it the way to count zero, which a group in the projection or a rule's head can't. Over no bindings the value is what an aggregate over nothing answers: `count` and `sum` 0, `min` and `max` absent, and `list` empty. It works in a rule as well as in the goal, so `util`, which imports nothing, has no heaviest import:

{{ demo "demos/guide/aggregation-body-rule.yaml" }}

The braces read only relations that are complete before the clause, as a head aggregate's body does, so a relation can't aggregate over itself. A variable they share has to be bound outside them by a relation that doesn't need the value, and inside them by a relation too. A variable written only inside them is theirs alone, so two aggregates in one body can each use `?t`. An aggregate can't sit inside another's braces yet.

## An aggregate in a rule

A rule's head can aggregate, as in `fan_in(?q, count(?p)) :- imports(?p, ?q);`, which gives the result a name other rules can read. The [aggregation tutorial]({{.Site.PathPrefix}}/tutorials/04-aggregation/#an-aggregate-in-a-rule) shows one. Two things follow from how jaala evaluates it:

- It has to be its relation's only rule, since two rules each producing a count for the same key would give the key two counts.
- Its body sits in a lower stratum than anything that reads the relation, the same as for `not`, so the count is never taken over a relation that's still growing. jaala refuses a cycle through an aggregating rule for the same reason it refuses one through `not`.

Its aggregate column reports a type whenever the function decides one: a number for `count`, text for `list`, and a number for `sum`, `min` and `max` over a column already typed as a number. Over an untyped column those three stay untyped.
