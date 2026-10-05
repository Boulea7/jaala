package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	queryErrRe   = regexp.MustCompile(`"query: ((?:[^"\\]|\\.)*)"`)
	unknownMsgRe = regexp.MustCompile(`fmt\.Sprintf\("((?:unknown|%q is a module)(?:[^"\\]|\\.)*)"`)
	fmtVerbRe    = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z]`)
)

// engineMessages is every "query: …" format string in jaala's production source, plus the unknown-
// name messages ns.Unknown builds for a "query: %s" wrapper to carry.
func engineMessages(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range []string{"../datalog", "../ns", "../stdlib"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src := read(t, f)
			res := []*regexp.Regexp{queryErrRe}
			if strings.HasSuffix(f, "suggest.go") {
				res = append(res, unknownMsgRe)
			}
			for _, re := range res {
				for _, m := range re.FindAllStringSubmatch(src, -1) {
					out[m[1]] = f
				}
			}
		}
	}
	return out
}

// stablePiece is a message format's longest run of fixed wording: what a host matching on fragments
// leans on, and what the catalogue has to show.
func stablePiece(format string) string {
	format = strings.ReplaceAll(format, `\"`, `"`)
	var best string
	for _, p := range strings.Split(fmtVerbRe.ReplaceAllString(format, "…"), "…") {
		if p = strings.Trim(p, ` :;(),"`); len(p) > len(best) {
			best = p
		}
	}
	return best
}

// Every error message in jaala's source is listed on the error catalogue, so a message added to the
// engine fails the docs build until it's documented, and one reworded fails until the page follows.
// A format that is only a wrapper ("%s") has no wording of its own to list.
func TestErrorCatalogueListsEveryMessage(t *testing.T) {
	page := read(t, "content/reference/errors.md")
	msgs := engineMessages(t)
	if len(msgs) < 90 {
		t.Fatalf("control: found only %d messages in jaala's source", len(msgs))
	}
	for format, file := range msgs {
		piece := stablePiece(format)
		if piece == "" {
			continue
		}
		if !strings.Contains(page, piece) {
			t.Errorf("%s: %q isn't on the error catalogue (content/reference/errors.md)", file, "query: "+format)
		}
	}
}
