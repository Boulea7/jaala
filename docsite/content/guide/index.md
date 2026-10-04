---
title: "Guide"
description: "The rules of jaala's query language, one topic per page, each shown by an example you can edit."
---

The [tutorials]({{.Site.PathPrefix}}/tutorials/) teach the language in order. These pages are where you look things up afterwards, and each one covers a single topic in full, including what jaala refuses and why. Like everything on this site, every example runs on jaala when the site is built, and you can edit and rerun it in the page. They all use the same [fact set]({{.Site.PathPrefix}}/tutorials/#the-facts) as the tutorials.

1. [Syntax](syntax/) lists every form a query can take.
2. [Types and constants](types-and-constants/) covers how a constant is read, and how numbers and text compare.
3. [Negation](negation/) covers anchoring, existential variables, host-bound values and stratification.
4. [Aggregation](aggregation/) covers groups, rows against values, `having`, and what an aggregate answers over nothing.
5. [Ordering](ordering/) covers the default order, `order by`, `limit` and `offset`.
6. [Host options](host-options/) covers `Bind`, `Budget` and the context, from Go.
7. [Modules and the vocabulary](modules-and-vocabulary/) covers paths, private members, docs, signatures and module errors.
8. [Absent values](absent-values/) covers a field the source didn't state.

The last three are written in Go, as `Example` functions that `go test` runs, like the [Go tutorial]({{.Site.PathPrefix}}/tutorials/05-go-host/).
