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
what a rule is and show it with an example. Before a docs PR is done, run `/ai-tell-sweep` and
`/prose-pass` (technical register) over the pages it touches.

The site documents `main`, not a release, and the footer names the commit it was built from.
