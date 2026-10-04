---
title: "Modules and the vocabulary"
description: "How a host names everything a query can call: modules of rules by path, private members, docs and signatures, and the errors Check reports."
prev: {url: "/jaala/guide/host-options/", title: "Host options"}
next: {url: "/jaala/guide/absent-values/", title: "Absent values"}
---

The vocabulary is the tree of names a query can call. Its leaves are the relations a source serves, the predicates a host computes (like `str.contains` from the standard library), and the members of modules, which are rules a host ships as a library. A query naming `deps.depends_on` pulls in the module that defines it, and whatever that module reads, so a library can be fairly big without every query paying for all of it.

These examples use the vocabulary from the [host options]({{.Site.PathPrefix}}/guide/host-options/) page, whose `deps` module defines `depends_on` with the help of a private `_step`.

```go
{{ includeFileText "examples/guide/modules_test.go" }}
```

## Paths and private members

A module lives at a path, and its members are named under it: `deps.depends_on`. Inside a module's own rules a bare name means that module's member first and the root's second, so rules don't repeat their module's path. A member whose name starts with `_` is private. The module's rules can call it, while a query can't, and `Lookup` leaves it out of the module's members, so a library can keep its helpers to itself.

## Docs and signatures

The comment lines above a member's first rule are its doc, and `Lookup` returns it with the member's signature and the origin the host gave when adding the module, usually a file name. A signature lists the member's arguments with their types, declared in a rule head or inferred through its rules, which is what a host shows when it lists what's available to query.

## Mistakes in a module

`Check` resolves every module once, after the host has added them, and refuses a module with a mistake in it before any query runs. The error reads as it would in a query, and it's a `*ns.ModuleError`, which carries the module's path and origin. So a host that loads a whole directory of `.dl` files (`AddModulesFS`) can say which file to fix.
