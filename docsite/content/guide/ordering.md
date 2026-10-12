---
title: "Ordering"
description: "The order every answer comes back in, and order by, limit and offset for when you want another."
prev: {url: "/jaala/guide/aggregation/", title: "Aggregation"}
next: {url: "/jaala/guide/host-options/", title: "Host options"}
---

Every answer comes back sorted, and the same facts and query always give the same order. Hosts lean on that quite a bit, since it lets them compare two answers line by line and check a saved answer against a new one.

## The default order

jaala sorts by the first column, then the second, and so on. Within a column, absent values come first, then numbers by value, then text as text. Duplicate rows collapse into one, since an answer is a set.

{{ demo "demos/guide/ordering-default.yaml" }}

## `order by`

`order by` names one or more answer columns, each with `asc` (the default) or `desc`, and the default order breaks any remaining ties. Here the rows sort by team, then by size within a team, largest first:

{{ demo "demos/guide/ordering-two-keys.yaml" }}

You name an aggregate column the way the projection writes it:

{{ demo "demos/guide/ordering-aggregate.yaml" }}

You can only name columns of the answer. A variable the projection leaves out isn't in the answer to sort by, so jaala refuses it rather than sorting by something you can't see:

{{ demo "demos/guide/ordering-unselected.yaml" }}

## `limit` and `offset`

`limit` with an `order by` keeps the first so many rows of that order, and `offset` skips some first, which together give you a page of the answer. These are the fourth to sixth largest packages:

{{ demo "demos/guide/ordering-limit-offset.yaml" }}

The query holds only the rows the page needs while it runs, `limit` plus `offset` of them, so a big answer's first page costs memory for that page. It still costs the work of the whole answer, since the last row found might belong on the page ([#194](https://github.com/panyam/jaala/issues/194)).

`limit` without an `order by` asks for any so many rows, and gets the first ones the query finds, so it stops there instead of working out the rest. The rows still come back in the default order, but which rows they are depends on how jaala happens to evaluate the query, so write an `order by` whenever it matters which ones you get:

{{ demo "demos/guide/ordering-limit-any.yaml" }}

That's also why `offset` needs an `order by`: a second page has to continue where the first one stopped, which needs an order that stays put.

{{ demo "demos/guide/ordering-offset-needs-order.yaml" }}

`limit` takes a positive whole number, so `limit 0` is refused rather than answering nothing, while `offset` takes any whole number, 0 included:

{{ demo "demos/guide/ordering-limit-zero.yaml" }}

## Paging with `after`

`after` continues from a row by its values rather than by counting: it keeps the rows strictly after those values in the `order by`, one value per `order by` column. So the page after the one above starts after `auth`'s 800 lines:

{{ demo "demos/guide/ordering-after.yaml" }}

A page that way doesn't recount what came before it, and doesn't shift when facts change between pages, which an `offset` does. The `order by` has to name every plain column, so no row ties with the one a page continues from and gets skipped. A host passes the last row it got to `datalog.After` instead of writing the values in, and `datalog.Total` says how many rows the whole answer has (see [host options]({{.Site.PathPrefix}}/guide/host-options/#pages-totals-and-tables)).

`order by`, `after`, `limit` and `offset` apply last, after any aggregation and `having`, against the columns as the projection writes them.
