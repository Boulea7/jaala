package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// TestEveryContentPageTemplateParses guards against a page that publishes blank. Content is run
// through text/template before markdown, so a stray "{{" anywhere, even inside a code fence, makes
// the whole page fail to render while the build exits 0. In a Go sample, put a composite literal's
// inner brace on its own line. Parsing with the site's own func map means a real directive passes
// and only an unknown function or a malformed action fails.
func TestEveryContentPageTemplateParses(t *testing.T) {
	funcs := template.FuncMap{}
	for name, fn := range Site.CommonFuncMap {
		funcs[name] = fn
	}

	var checked int
	err := filepath.Walk("content", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		if _, err := template.New(filepath.Base(path)).Funcs(funcs).Parse(string(body)); err != nil {
			t.Errorf("%s would publish as a BLANK page: %v\n"+
				"a stray {{ is parsed as a template action; in a Go sample put the element brace on its own line", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk content: %v", err)
	}
	if checked == 0 {
		t.Fatal("no content pages found, so this guard asserted nothing")
	}
	t.Logf("template-parsed %d content pages", checked)
}
