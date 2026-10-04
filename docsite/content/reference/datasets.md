---
title: "Datasets"
description: "Every fact set the site's examples run on, relation by relation."
---

Each example on this site names the facts it runs on, and its caption links here. These tables are read from the fixture files the examples use, when the site is built, so they're exactly the facts behind every answer you see.

## graph

{{ dataset "graph" }}

The [overview]({{.Site.PathPrefix}}/overview/) uses this one.

## deps

{{ dataset "deps" }}

The [tutorials]({{.Site.PathPrefix}}/tutorials/), the [guide]({{.Site.PathPrefix}}/guide/) and the rest of the reference use this one. `cache` and `config` import each other on purpose, so the recursion examples have a cycle to get through.
