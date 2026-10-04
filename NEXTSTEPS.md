# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #104 `mission_docsite`, 2/7: the docsite and its build-time demos are in (#113, #114,
  #115). Next ready: #107 (live demos in wasm; plan it first, measuring the wasm size and whether
  TinyGo is worth trying), then #108 tutorials. Exercise `make -C docsite check` last ran at
  856c39e, 1 PR (#115, CSS) since.
- Mission #86 `mission_selfcheck`, 8/11: what's left is #22 (canonical citations, the last known
  disagreement), #23 (Soufflé) and #89 (fuzzing), all P2. `./selfcheck.sh` last ran at b98b2d1,
  3 docs PRs since, none touching the engine. v0.1.21 is the latest tag.
- Mission #81 looks done: agni#819 closed through agni#850 (`net.test_point_count`), and agni#843
  closed with its exercise passing at agni 173b859c. Waiting on the owner's go-ahead to log that on
  #81 and close it.
- No open threads. This run dropped every branch from the selfcheck and docsite work (PRs #94,
  #95, #98–#103, #113–#115 merged) and the old `checkpoint-v0.1.19` thread (#85 merged).

## Across threads

- GitHub Pages isn't enabled for the repo, so `docs.yml`'s deploy job fails on every push to main
  while its build passes. The owner sets Settings, Pages, Source to "GitHub Actions"; the next
  merge then publishes https://panyam.github.io/jaala/.
