---
title: "Grammar"
description: "The query language's grammar in EBNF, read from the parser's source when the site is built."
---

This is the grammar `datalog.Parse` reads, copied from the comment on `Parse` in `datalog/parse.go` when the site is built, so it's the parser's own description of itself. A `#` outside a string starts a comment that runs to the end of the line, and whitespace is insignificant.

```text
{{ grammar }}
```

## Every clause at once

One query using each part of the grammar: a rule with a comment and a declared type, then a goal with a join, a negation and a comparison, and a projection with aggregates, `having`, `order by`, `limit` and `offset`.

{{ demo "demos/reference/grammar-every-clause.yaml" }}

The [syntax]({{.Site.PathPrefix}}/guide/syntax/) page of the guide walks through the same forms one at a time.
