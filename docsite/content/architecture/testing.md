---
title: "Testing"
description: "How jaala checks its own answers, its own cost, and these docs, and how its tests are checked in turn."
prev: {url: "/jaala/architecture/provenance/", title: "Provenance"}
---

jaala's hosts trust its answers enough to act on them, so we test most answers against a reference rather than against what a test author expected. Four layers of checks run on every change.

## Every query, three ways

Every test that evaluates a query runs it through `Naive`, `SemiNaive{WrittenOrder: true}` and the planned `SemiNaive{}`, and fails if they disagree. Written order has to match Naive's rows and errors exactly, and its citations wherever the program can't derive a tuple two ways. The planned evaluator has to match Naive's rows, and give Naive's error whenever it fails. So a new optimisation is tested by every test that already exists, the moment it lands.

## Generated programs

Hand-written tests only reach the shapes their authors thought of. A generator writes random programs over random graphs, with recursion, negation, aggregates, constants, bound goals and `Bind`, and runs each through all three evaluators, plainly and under `Witnesses()`. The test shrinks a disagreement to a small program and graph before it reports it, which makes a report pretty quick to read. Its first run found four bugs the hand-written tests had missed ([#87](https://github.com/panyam/jaala/issues/87)). The test lists known, filed disagreements, each with a program that reproduces it, so a fixed one shows up as fixed. CI runs a few hundred programs on every change, and `./selfcheck.sh` runs thousands.

## Work baselines

Answers being right says nothing about cost. Twelve workloads (closures, same-generation, a points-to analysis, a netlist the size of an agni board) record the work the planned evaluator does, and a test fails when any of them moves more than 10% either way ([#88](https://github.com/panyam/jaala/issues/88)). Work is counted, not timed, so it's the same on every run and on every machine. The baselines found two costs worth fixing as soon as they existed ([#96](https://github.com/panyam/jaala/issues/96), [#97](https://github.com/panyam/jaala/issues/97)).

## The docs

This site is checked the same way. Every example runs on the engine when the site is built, pins its answer or its error, and fails the build if the engine disagrees. The live editor's engine is tested as wasm under Node. The [error catalogue]({{.Site.PathPrefix}}/reference/errors/) is checked against every message in jaala's source, and every internal link and anchor in the built site is followed. Writing these pages found bugs too: a panic on a goal with too few arguments ([#133](https://github.com/panyam/jaala/issues/133)), an internal name in an error ([#127](https://github.com/panyam/jaala/issues/127)), and blank aggregates ([#122](https://github.com/panyam/jaala/issues/122)).

## Tests that can fail

A test that can't fail checks nothing, so every new test is red-checked: break the behaviour it guards, and confirm it fails on its assertion, not on a build error. Where a fixture might not tell the cases apart, the test carries a control that proves it does, for example that a workload is big enough for a cost regression to show. To find what the tests miss, we inject bugs into the rewrites on purpose. One that reused variable names across inlined calls passed every hand-written test, and the generated programs caught it.
