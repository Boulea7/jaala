package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	linkRe = regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	idRe   = regexp.MustCompile(`\sid="([^"]+)"`)
)

// TestBuiltLinksResolve follows every internal link and asset in the built site, ./dist, and fails on
// one that points at no file, or at an anchor the target page doesn't have. Hand-written navs and
// cross-page links drift as pages move, and a broken one still builds. It needs `make build` first,
// which `make test` and `make check` do.
func TestBuiltLinksResolve(t *testing.T) {
	if _, err := os.Stat("dist/index.html"); err != nil {
		t.Fatal("dist/ isn't built: run `make build` (or `make check`, which builds and tests)")
	}
	ids := map[string]map[string]bool{} // built file -> its element ids
	idsOf := func(file string) map[string]bool {
		if got, ok := ids[file]; ok {
			return got
		}
		got := map[string]bool{}
		if b, err := os.ReadFile(file); err == nil {
			for _, m := range idRe.FindAllStringSubmatch(string(b), -1) {
				got[m[1]] = true
			}
		}
		ids[file] = got
		return got
	}
	var pages, links int
	err := filepath.Walk("dist", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") || strings.HasPrefix(path, "dist/static/") {
			return err
		}
		pages++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range linkRe.FindAllStringSubmatch(string(b), -1) {
			target, anchor, _ := strings.Cut(m[1], "#")
			var file string
			switch {
			case target == "" && anchor != "":
				file = path
			case strings.HasPrefix(target, PathPrefix+"/") || target == PathPrefix:
				file = filepath.Join("dist", strings.TrimPrefix(target, PathPrefix))
				if strings.HasSuffix(target, "/") || target == PathPrefix {
					file = filepath.Join(file, "index.html")
				}
			default:
				continue // external, or a relative link the site doesn't write
			}
			links++
			if _, err := os.Stat(file); err != nil {
				t.Errorf("%s links %q, but %s doesn't exist", path, m[1], file)
				continue
			}
			if anchor != "" && !idsOf(file)[anchor] {
				t.Errorf("%s links %q, but %s has no id %q", path, m[1], file, anchor)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk dist: %v", err)
	}
	if pages == 0 || links == 0 {
		t.Fatalf("checked %d pages and %d links, so this asserted nothing", pages, links)
	}
	t.Logf("checked %d internal links across %d pages", links, pages)
}
