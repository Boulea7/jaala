# jaala's documentation site

The site at https://panyam.github.io/jaala/, built with [s3gen](https://github.com/panyam/s3gen).
This directory is its own Go module, so s3gen and its dependencies never reach jaala's, which stays
standard-library only and builds for wasm. Its layout and tests follow agni's docsite.

## Commands

```sh
make -C docsite run     # serve at http://localhost:8085/jaala/ (builds once; restart after an edit)
make -C docsite build   # build into docsite/dist
make -C docsite check   # build, then run every docsite test: the mission's exercise (#104)
```

`.github/workflows/docs.yml` runs `make check` on every PR and deploys `dist/` to Pages from `main`.

## Layout

- `content/` holds the pages, as Markdown with YAML front matter (`title`, `description`). A
  directory is a section, and its `index.md` is the section's landing page.
- `content/HeaderNavLinks.json` is the header nav, and `content/SiteMetadata.json` the site's name,
  description and links.
- `templates/` holds the page layout: `BasePage.html` with `Header`, `Sidebar`, `Content` and
  `Footer`, and one `nav/<Section>Nav.html` per section for its sidebar.
- `static/` is copied into the build as is: the stylesheet and the scripts for code blocks and the
  header dropdowns.
- `main.go` configures s3gen and the template functions pages can call.

## Adding a page

1. Write `content/<section>/<slug>.md` with a `title` and a `description`.
2. Link it from `content/<section>/index.md`.
3. Add it to `templates/nav/<Section>Nav.html`.
4. If the section has a header dropdown, add it to `content/HeaderNavLinks.json`.

A new section also needs its nav template included at the top of `templates/Sidebar.html` and a
`Contains $currentPath "/<section>/"` branch dispatching to it, plus an entry in the header nav.
`nav_test.go` checks each of these, so a missed edit fails `make check`.

## Examples

Every example on the site runs. A page shows one with `{{ demo "demos/<section>/<name>.yaml" }}` on
a line of its own, and the build runs it on jaala and writes the rules, the goal and the answer table
into the page. A spec looks like this:

```yaml
fixture: graph                # a fact set in demo/fixtures/, or leave it out
facts: |                      # more facts, added to the fixture's
  edge("d", "a")
program: |                    # the rules
  reach(?a, ?b) :- edge(?a, ?b);
  reach(?a, ?c) :- reach(?a, ?b), edge(?b, ?c);
query: reach("a", ?x) => ?x   # the goal
bind: {x: "c"}                # goal variables the host binds, as datalog.Bind does
cites: true                   # show each row's citations
expect: [[b], [c], [d]]       # the rows it must answer, in order
expect_error: "not stratifiable"   # or: the error it must fail with
```

Facts are ground atoms, `rel("text", 3)`, one per line or separated by `;`. jaala has no fact syntax
of its own, since hosts serve facts from Go, so `demo/facts.go` is the docsite's. A fact's text is its
citation.

An example that pins rows or an error must produce exactly that, and one that pins nothing must at
least run without error. Otherwise the build and `demos_test.go` both fail, naming the spec.
Pin whatever the prose around an example claims, so the two can't drift apart. Every spec has to
be shown on some page.

`demo.Run` is the one function that runs an example: the build, the tests and the in-page editor
(#107) all call it, with a work budget so an edited example that runs away stops instead of hanging.

## Things that publish a broken page without failing the build

- **A stray `{{`.** Pages are run through Go's `text/template` before Markdown, so `{{` anywhere,
  even in a code fence, makes the whole page render blank. In a Go sample, put a composite literal's
  inner brace on its own line. `template_test.go` catches it.
- **A link to a page or anchor that isn't there.** Write internal links as
  `{{.Site.PathPrefix}}/section/page/`, with the trailing slash. `links_test.go` checks every one in
  the built site, anchors included.
- **A figure include with a wrong path.** `includeFile` returns nothing for a path that doesn't
  resolve. `includefile_test.go` checks every include.

## Writing

The same rules as the rest of jaala's prose: plain declarative sentences, "we" for the project, no
em-dashes, no colon-definitions ("The result: X"), no hype adjectives, no "not just X but Y". Say
what a rule is and show it with an example. Before a docs PR is done, run `/ai-tell-sweep` and then
`/prose-pass` (technical register) over the pages it touches, and say in the PR what they changed.

## Before and after screenshots

A PR that changes how a page looks carries screenshots of the changed section, before and after,
in both themes. The checks can't see layout: the first set caught a border drawn inside the code
block's wrapper (#115). Serve the base commit from a worktree and the branch side by side, with
`JAALA_DOCS_PORT=:8091 go run .` and `:8092`, then shoot each with Playwright (`fullPage: true`,
clipped to the section, with `localStorage.theme` set to `dark` and to `light`). Stop the servers by
port, `fuser -k 8091/tcp 8092/tcp`, since a `pkill -f` pattern also matches the shell running it.

The site documents `main`, not a release, and the footer names the commit it was built from.
