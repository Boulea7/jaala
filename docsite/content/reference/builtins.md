---
title: "Built-ins"
description: "Everything the standard library adds to a vocabulary: string tests and absent, read from its registry."
---

`stdlib.Register` adds these to a vocabulary. The table is built from the registry itself when the site is built, with each name's signature and the doc it was registered with.

{{ builtins }}

Each one is a test: it holds or it doesn't, and it binds nothing. So every argument needs a value from the rest of the goal, pretty much the way a comparison does. jaala's planner runs a test once its arguments are bound, wherever you wrote it in the goal, and refuses one whose arguments nothing binds:

{{ demo "demos/reference/builtins-unbound.yaml" }}

## String tests

`str.contains`, `str.prefix` and `str.suffix` compare plain substrings:

{{ demo "demos/reference/builtins-contains.yaml" }}

{{ demo "demos/reference/builtins-prefix.yaml" }}

{{ demo "demos/reference/builtins-suffix.yaml" }}

`str.glob` matches the whole string against a SQLite-style glob, where `*` is any run of characters, `?` one character, and `[a-c]` one of a class:

{{ demo "demos/reference/builtins-glob.yaml" }}

`str.match` takes a regular expression in Go's syntax, unanchored, so anchor it with `^` and `$` to match the whole string:

{{ demo "demos/reference/builtins-match.yaml" }}

A pattern that doesn't compile is refused before anything runs, with the compiler's reason:

{{ demo "demos/reference/builtins-bad-regex.yaml" }}

{{ demo "demos/reference/builtins-bad-glob.yaml" }}

Go code that has to agree with these (a host filtering the same names outside a query, say) can mostly reuse them through `stdlib.CompileGlob` and `stdlib.CompilePattern`, the compilers the tests use.

## `absent`

`absent(?x)` holds when `?x` is a field its source didn't state. None of `deps`'s fields are absent, so here it holds for nothing, and the [guide]({{.Site.PathPrefix}}/guide/absent-values/) has an example over facts that do leave fields out:

{{ demo "demos/reference/builtins-absent.yaml" }}
