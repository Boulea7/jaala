# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #86 `mission_selfcheck`, 10/13: #133 and #127 closed by #135 (arity checked as written,
  plus `TestBrokenProgramsFailTheSameWay`, #89's first step). Next ready: #22 (canonical citations,
  the last known disagreement), #23 (Soufflé), #89 (fuzzing the text), all P2. `./selfcheck.sh` last
  logged at a8481f9; #136 merged since, and it passed on that branch too (168/168 set-bound goals).
- Mission #104 `mission_docsite`, 7/7: exercise logged at cd4b594 and run on each PR since. Waiting
  on the owner's go-ahead to close it.
- Release: v0.1.21 is the latest tag. v0.1.22 goes on 34757bf (#135, #136). #136 breaks hosts
  (`Bind` takes `map[Var][]ns.Value`), so after tagging, post the upgrade note on #132 with the
  lines agni and Declaire change, and note the tag on #133 and #127.
- Open: draft PR #137 (outside contributor, for #129, empty comma pieces), waiting on its author's
  prose checks before review. #126 (P2) serves no mission.
- The site is live at https://panyam.github.io/jaala/ and every merge to main redeploys it.
- No open threads. This run dropped the unmerged `checkpoint-docsite` commit b95206f (mission #81,
  since closed) and every branch from #135 and #136.
