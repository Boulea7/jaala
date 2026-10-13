---
title: "Arithmetic and units"
description: "Expressions in comparisons, ?y = expr to compute a value, functions that run in any direction, and how units multiply, divide and get their names."
prev: {url: "/jaala/guide/types-and-constants/", title: "Types and constants"}
next: {url: "/jaala/guide/negation/", title: "Negation"}
---

A comparison can compute: `?w * 2 > ?limit`, or `?y = expr`, which gives `?y` the expression's value ([#5](https://github.com/panyam/jaala/issues/5)). These use resistors with a resistance, a current and the rail they sit on:

{{ demo "demos/guide/arithmetic-product.yaml" }}

## Functions run in any direction

Arithmetic is written in functions the standard library registers: `num.add`, `num.sub`, `num.mul`, `num.div` and `num.neg`. Each is a relation of its operands and its result, so `?v = ?i * ?r` is `num.mul(?i, ?r, ?v)`, and it runs from whichever two it has. Given the voltage and the current, the same expression finds the resistance, which is Ohm's law used in either direction:

{{ demo "demos/guide/arithmetic-solve.yaml" }}

Where the expression is written doesn't matter: a step waits until what it needs is bound, as a comparison does. Expressions nest with the usual precedence and parentheses:

{{ demo "demos/guide/arithmetic-parallel.yaml" }}

and can call a function by name, in which case the function's last argument is its result, so `num.div(?r, 2)` is `?r / 2`. A host's own functions work the same way, with `name(args)` calling the relation `name` with one more argument for the result:

{{ demo "demos/guide/arithmetic-function.yaml" }}

A division by zero, or a value that isn't a number, has no result, so that row has none rather than the query failing.

## Units

A number can carry a unit, and a query can write one after a number: `1kohm`, `3.3V`, `0.5A`. A prefix is folded in, so `1kohm` is 1000 ohm and compares with a source's ohms:

{{ demo "demos/guide/arithmetic-literal.yaml" }}

A product's unit multiplies and a quotient's divides, and the result takes a unit's name when the vocabulary has one: amps times ohms is volts, above. Adding or comparing needs one unit, and a bare number takes the other side's. Volts plus amps has no answer:

{{ demo "demos/guide/arithmetic-unlike.yaml" }}

A unit nobody named is still a unit, written as what it is:

{{ demo "demos/guide/arithmetic-composite.yaml" }}

The standard library names the SI prefixes and an electrical set (A, V, ohm and Ω, W, J, C, F, H, Hz), and a host adds its own with `Vocabulary.AddUnit`: a base unit, a prefix, or a name for a product, such as `speed` for `m/s`. A unit's first name is how answers write it. A literal's unit is letters only, so a compound one is written as arithmetic, `3m / 1s`.

A source still serves its values in base units, so `5mV` arrives as `0.005V`. A source serving prefixed values, compared by what they mean, is [#205](https://github.com/panyam/jaala/issues/205).

## What arithmetic can't do yet

A rule can't compute a value and feed it back into its own recursion, since `n(?m) :- n(?n), ?m = ?n + 1` never stops:

{{ demo "demos/guide/arithmetic-recursion.yaml" }}

Arithmetic in a rule's head or in the projection (`=> ?a * ?b`) is [#206](https://github.com/panyam/jaala/issues/206); compute into a variable in the body and project that.
