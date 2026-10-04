---
title: "Rules and recursion"
description: "Derived relations, and a rule that follows imports all the way down, through a cycle."
prev: {url: "/jaala/tutorials/01-first-query/", title: "A first query"}
next: {url: "/jaala/tutorials/03-negation/", title: "Negation"}
---

A rule defines a new relation from the ones you have. It reads right to left: the part after `:-` is a goal, and for each way it matches, the head before `:-` holds. This one says that `a` imports `b` within a team when `a` imports `b` and the same team owns both:

{{ demo "demos/tutorials/02-same-team.yaml" }}

A rule ends with `;`, and a query is any number of rules followed by one goal. The relation a rule defines can be used in a goal, or in another rule, exactly like one the facts provide. jaala doesn't know `same_team` until a rule says what it is, so a query that names a relation with no rules and no facts behind it is refused:

{{ demo "demos/tutorials/02-no-rules.yaml" }}

## Recursion

`imports` only says what a package imports directly. What a package depends on is what it imports, plus what those import, and so on down. That's two rules, and the second one uses the relation it defines:

{{ demo "demos/tutorials/02-depends-on.yaml" }}

The first rule covers the direct imports. The second says that if `a` depends on `b`, and `b` imports `c`, then `a` depends on `c` too. jaala applies both rules until a round derives nothing new, which is the fixpoint, so it doesn't really matter how deep the chain goes.

## Cycles

`cache` and `config` import each other, which would send a naive walk of the graph round in circles. A rule can't, because jaala only adds facts it didn't have, and there are finitely many. So `cache` depends on `config`, and through it on itself:

{{ demo "demos/tutorials/02-cycle.yaml" }}

## Asking why

A goal with no variables asks whether something holds. When it does, the answer is one row with no columns, and its citations say why. `app` depends on `util` because `app` imports `log`, and `log` imports `util`:

{{ demo "demos/tutorials/02-why.yaml" }}

`app` reaches `util` along several paths, and a row cites one of them, the derivation that found it first. That's mostly what you want from evidence: one path you can check, not every path there is.

## The other direction

The same rules answer the opposite question, too. Put the constant in the second position and you get everything that depends on `util`, which here is every package but `util` itself:

{{ demo "demos/tutorials/02-dependents.yaml" }}

You don't write the rules differently for this. jaala works out which arguments a goal fixes, and uses that to avoid deriving what the question doesn't need.
