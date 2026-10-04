---
title: "Ordering"
description: "The order every answer comes back in, and order by, limit and offset for when you want another."
prev: {url: "/jaala/guide/aggregation/", title: "Aggregation"}
---

Every answer comes back sorted, and the same facts and query always give the same order. That's a promise hosts lean on: two answers can be compared line by line, and a saved answer can be checked against a new one.

## The default order

jaala sorts by the first column, then the second, and so on. Within a column, absent values come first, then numbers by value, then text as text. Duplicate rows collapse into one, since an answer is a set.

{{ demo "demos/guide/ordering-default.yaml" }}

## `order by`

`order by` names one or more answer columns, each with `asc` (the default) or `desc`, and the default order breaks any remaining ties. Here the rows sort by team, then by size within a team, largest first:

{{ demo "demos/guide/ordering-two-keys.yaml" }}

An aggregate column can be named the way the projection writes it:

{{ demo "demos/guide/ordering-aggregate.yaml" }}

Only columns of the answer can be named. A variable the projection leaves out isn't in the answer to sort by, so jaala refuses it rather than sorting by something you can't see:

{{ demo "demos/guide/ordering-unselected.yaml" }}

## `limit` and `offset`

`limit` keeps the first so many rows and `offset` skips some first, both applied after the order, which together give you a page of the answer. These are the fourth to sixth largest packages:

{{ demo "demos/guide/ordering-limit-offset.yaml" }}

`limit` takes a positive whole number, so `limit 0` is refused rather than answering nothing, while `offset` takes any whole number, 0 included:

{{ demo "demos/guide/ordering-limit-zero.yaml" }}

`order by`, `limit` and `offset` apply last, after any aggregation and `having`, against the columns as the projection writes them.
