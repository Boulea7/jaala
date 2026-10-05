---
title: "Built-ins"
description: "Everything the standard library adds to a vocabulary: string tests, edit distance and absent, read from its registry."
---

`stdlib.Register` adds these to a vocabulary. The table is built from the registry itself when the site is built, with each name's signature and the doc it was registered with.

{{ builtins }}

All but `str.distance` are tests: each holds or it doesn't, and binds nothing. So every argument needs a value from the rest of the goal, pretty much the way a comparison does. jaala's planner runs a test once its arguments are bound, wherever you wrote it in the goal, and refuses one whose arguments nothing binds:

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

Go code that has to agree with these (a host filtering the same names outside a query, say) can use `stdlib.CompileGlob` and `stdlib.CompilePattern`, the compilers the tests use.

## Edit distance

`str.distance(?a, ?b, ?d)` binds `?d` to the edit distance between two strings: the fewest characters inserted, deleted or replaced to turn one into the other (Levenshtein's distance). A character is a Unicode character, so `Ω` against `O` is one edit. Both strings need values from the rest of the goal, and `?d` can be a variable it binds or a number it tests. Two names a character apart are often one name misspelled, so this finds them:

{{ demo "demos/reference/builtins-distance.yaml" }}

Each pair is measured once per binding, so a goal comparing every name with every other does work for every pair. Matching is exact on case: `EN` and `en` are two edits apart.

With neither string bound there is nothing to measure, and the goal is refused:

{{ demo "demos/reference/builtins-distance-unbound.yaml" }}

## `absent`

`absent(?x)` holds when `?x` is a field its source didn't state. None of `deps`'s fields are absent, so here it holds for nothing, and the [guide]({{.Site.PathPrefix}}/guide/absent-values/) has an example over facts that do leave fields out:

{{ demo "demos/reference/builtins-absent.yaml" }}
