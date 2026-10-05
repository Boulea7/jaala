---
title: "Layering"
description: "The three packages, ns, stdlib and datalog, and why each imports only the one below it."
next: {url: "/jaala/architecture/evaluation/", title: "Evaluation"}
---

jaala is three Go packages, and they import each other in one direction only:

- **`ns`** is the contract. It holds the vocabulary's tree of names, values and their types, signatures, the `Source` a host serves facts through, and the hook that lets a language like Datalog define modules, and it imports nothing else in jaala.
- **`stdlib`** is the standard vocabulary, with the string tests, `absent`, and the glob and regex compilers. It imports `ns` and never the engine.
- **`datalog`** is the engine, which parses, links, checks, infers and evaluates. It imports `ns` only, so even the standard library is something a host adds rather than something the engine assumes.

We keep the direction strict for a fairly practical reason. A host's fact layer, the code that turns a netlist or a repository into facts, imports `ns` to describe its relations, and it shouldn't have to take on a query engine to do it. agni's own architecture depends on that split. A test at the module's root, `TestLayering`, fails if any import runs the wrong way.

## Standard library only

All three use only the Go standard library, and CI checks it on every change with `go list -deps`. Anything jaala imports, every host imports too, and agni runs jaala in the browser, where a dependency is download size. The same reason keeps everything building for `GOOS=js GOARCH=wasm`, which CI also checks, and which is how the examples on this site run in your browser.

This documentation site is its own Go module under `docsite/`, so the site generator and its dependencies never reach jaala's.
