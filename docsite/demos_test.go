package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/panyam/jaala/docsite/demo"
)

var demoRe = regexp.MustCompile(`\{\{-?\s*demo\s+"([^"]+)"`)

func demoSpecs(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("demos", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yaml") {
			out = append(out, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk demos: %v", err)
	}
	return out
}

// Every example runs as its spec says: its pinned rows, its pinned error, or at least no error. The
// build checks the same, page by page, so a failure here is one the build would refuse to publish.
func TestEveryDemoRunsAsItsSpecSays(t *testing.T) {
	specs := demoSpecs(t)
	if len(specs) == 0 {
		t.Fatal("no demo specs under demos/, so this asserted nothing")
	}
	for _, path := range specs {
		s, err := loadSpec(path)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		tab, runErr := demo.Run(s)
		if err := demo.Check(s, tab, runErr); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	t.Logf("ran %d demos", len(specs))
}

// Every spec is shown on some page, and every page's demo resolves to a spec, so an example can't
// sit unused or point at nothing.
func TestEveryDemoIsShownAndEveryShownDemoExists(t *testing.T) {
	shown := map[string]bool{}
	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		for _, m := range demoRe.FindAllStringSubmatch(read(t, path), -1) {
			shown[m[1]] = true
			if _, err := os.Stat(m[1]); err != nil {
				t.Errorf("%s shows demo %q, which doesn't exist", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk content: %v", err)
	}
	for _, path := range demoSpecs(t) {
		if !shown[path] {
			t.Errorf("%s isn't shown on any page", path)
		}
	}
}

// A demo that doesn't run as its spec says fails the build and says why on the page, rather than
// publishing a wrong answer. control: a correct demo renders its table and records nothing.
func TestABrokenDemoFailsTheBuild(t *testing.T) {
	reset := func() { demoFailures = nil }
	defer reset()
	for path, want := range map[string]string{
		"testdata/demos/wrong-expect.yaml":     "expected rows [[c]]",
		"testdata/demos/unexpected-error.yaml": "query:",
	} {
		reset()
		demoHTML(path)
		if len(demoFailures) != 1 || !strings.Contains(demoFailures[0], want) {
			t.Errorf("%s: recorded %q, want one failure containing %q", path, demoFailures, want)
		}
	}
	reset()
	if out := string(demoHTML("demos/overview/reach.yaml")); len(demoFailures) != 0 || !strings.Contains(out, "<td>d</td>") {
		t.Errorf("control: a correct demo recorded %q and rendered %s", demoFailures, out)
	}
}
