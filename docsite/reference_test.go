package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/jaala/docsite/demo"
)

// Every fixture an example can name has its section on the datasets page, which every caption
// naming it links to.
func TestEveryFixtureIsADataset(t *testing.T) {
	page := read(t, "content/reference/datasets.md")
	for _, name := range demo.Fixtures() {
		if !strings.Contains(page, "\n## "+name+"\n") || !strings.Contains(page, `{{ dataset "`+name+`" }}`) {
			t.Errorf("fixture %s has no `## %s` section showing {{ dataset %q }} on the datasets page", name, name, name)
		}
	}
}

// The grammar page shows the parser's own grammar. control: a file with no grammar block is an error,
// not an empty page.
func TestGrammarComesFromTheParser(t *testing.T) {
	g, err := extractGrammar(parserFile)
	if err != nil {
		t.Fatal(err)
	}
	var productions int
	for _, line := range strings.Split(g, "\n") {
		if strings.Contains(line, " = ") {
			productions++
		}
	}
	if !strings.HasPrefix(g, "query ") || productions < 20 {
		t.Errorf("extracted %d productions, starting %q", productions, g[:min(40, len(g))])
	}
	if _, err := extractGrammar("main.go"); err == nil {
		t.Error("control: a file with no grammar should be an error")
	}
}

// Every name the standard library registers has an example on the built-ins page.
func TestEveryBuiltinHasADemo(t *testing.T) {
	entries, err := builtins()
	if err != nil {
		t.Fatal(err)
	}
	var queries strings.Builder
	paths, _ := filepath.Glob("demos/reference/builtins-*.yaml")
	for _, p := range paths {
		s, err := loadSpec(p)
		if err != nil {
			t.Fatal(err)
		}
		queries.WriteString(s.Query + "\n")
	}
	if len(entries) < 6 {
		t.Fatalf("control: found only %d built-ins", len(entries))
	}
	for _, e := range entries {
		if !strings.Contains(queries.String(), e.Path+"(") {
			t.Errorf("built-in %s has no demo in demos/reference/builtins-*.yaml", e.Path)
		}
	}
}
