---
title: "Types and constants"
description: "How jaala reads a constant against the type of the argument it stands in, and how numbers and text compare and sort."
prev: {url: "/jaala/guide/syntax/", title: "Syntax"}
next: {url: "/jaala/guide/negation/", title: "Negation"}
---

Every value in jaala is a number or text (or absent, which the host-facing pages cover). A relation's arguments can have types, and when they do, jaala reads a constant as the type of the argument it stands in.

## Declared types

A rule head can declare an argument's type with `?n: number`. That turns a constant written in the query into a value of that type, so `"800"` in a number argument means the number 800:

{{ demo "demos/guide/types-declared-read.yaml" }}

And a constant that can't be read that way is refused, rather than answering nothing:

{{ demo "demos/guide/types-declared-refused.yaml" }}

The difference matters most when a question comes from a person or a form, where `"big"` is a mistake you'd want reported. Without the declaration, jaala has no type to check against, and the same question matches no rows without saying why:

{{ demo "demos/guide/types-undeclared.yaml" }}

A host can declare types on the relations it serves too, which mostly saves you writing them in rules, and a rule's undeclared arguments pick up types from what its body reads. The facts on this site declare none, which is why the rule here has to.

## Numbers and text

Numbers compare by value and text compares as text, which is the usual order for strings:

{{ demo "demos/guide/types-text-order.yaml" }}

A number and a piece of text have no order between them, so a comparison of one with the other never holds, whichever way round it's written:

{{ demo "demos/guide/types-number-vs-text.yaml" }}

When a column holds both, the answer still has to come back in one order, so jaala puts every number before every piece of text, numbers by value and text as text:

{{ demo "demos/guide/types-mixed-column.yaml" }}

That order is the same every time, for the same facts and the same query, which is what lets a host compare two answers line by line. The [ordering]({{.Site.PathPrefix}}/guide/ordering/) page has the rest of it.

An answer writes a plain number one way, the way Go's `%g` would, however the query or the source spelled it. So `1.50` comes back as `1.5`, `01` as `1`, and `1.0` and `1` are the same row ([#148](https://github.com/panyam/jaala/issues/148)):

{{ demo "demos/guide/types-number-spelling.yaml" }}

Text that says more than the number is kept as the source wrote it. A host storing a voltage as `3.3V` with the number 3.3 gets `3.3V` back, and so does a query that asked about `3.3`, since where two spellings of one number meet, the one saying more wins. A value the host bound with `Bind` comes back exactly as bound.
