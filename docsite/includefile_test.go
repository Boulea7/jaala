package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A diagram lives in figures/ and a page pulls it in with includeFile, so the SVG is inlined at build
// time and keeps `currentColor` and `--accent-color`, which an <img> can't. includeFile returns ""
// for a path that doesn't resolve, so a typo or a rename drops the figure while the build succeeds;
// these tests catch that.

var includeFileRe = regexp.MustCompile(`\{\{-?\s*includeFile\s+"([^"]+)"`)

const figuresDir = "figures"

// includedFiles returns every path included from content, keyed to the pages doing it.
func includedFiles(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range includeFileRe.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = append(out[m[1]], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", contentDir, err)
	}
	return out
}

func TestEveryIncludedFileResolves(t *testing.T) {
	for rel, pages := range includedFiles(t) {
		if _, err := os.Stat(filepath.Join(projectRoot, rel)); err != nil {
			sort.Strings(pages)
			t.Errorf("%s includes %q, which does not exist, so the page renders without it and nothing says so\n  included by: %s",
				filepath.Base(pages[0]), rel, strings.Join(pages, ", "))
		}
	}
}

// A figure nothing includes is a diagram someone drew and the prose never adopted, the same argument
func TestEveryFigureIsIncluded(t *testing.T) {
	included := includedFiles(t)
	entries, err := os.ReadDir(filepath.Join(projectRoot, figuresDir))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", figuresDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		rel := figuresDir + "/" + e.Name()
		if len(included[rel]) == 0 {
			t.Errorf("%s is not included by any page", rel)
		}
	}
}

// The figures directory exists so these stay theme-aware. A literal colour reads
// correctly in whichever theme it was authored in and badly in the other, and the docsite has both.
func TestFiguresCarryNoColourLiterals(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(projectRoot, figuresDir))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", figuresDir, err)
	}
	literal := regexp.MustCompile(`(?:fill|stroke|stop-color)="\s*(#[0-9a-fA-F]{3,8}|rgb\(|hsl\()`)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(projectRoot, figuresDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, m := range literal.FindAllString(string(b), -1) {
			t.Errorf("%s/%s carries the colour literal %q; use currentColor or var(--accent-color)",
				figuresDir, e.Name(), m)
		}
	}
}

// A blank line inside a figure ends the raw-HTML block the inlined SVG is. includeFile splices the
// file in before markdown runs, and CommonMark ends an HTML block at a blank line, so what follows
// can be parsed as a paragraph, wrapped in <p>, and close the <svg> early. The figure still renders,
// minus its tail.
func TestFiguresCarryNoBlankLines(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(projectRoot, figuresDir))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", figuresDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(projectRoot, figuresDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("%s/%s has a blank line at line %d; it ends the raw-HTML block, so everything after it can fall outside the <svg>",
					figuresDir, e.Name(), i+1)
				break
			}
		}
	}
}

var includeFileTextRe = regexp.MustCompile(`\{\{-?\s*includeFileText\s+"([^"]+)"`)

// Every Go example under examples/ is shown on some page, and every includeFileText path resolves.
// An example's own test keeps its code honest, and this keeps the page showing it.
func TestEveryExampleIsShown(t *testing.T) {
	shown := map[string]bool{}
	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range includeFileTextRe.FindAllStringSubmatch(string(b), -1) {
			shown[m[1]] = true
			if _, err := os.Stat(m[1]); err != nil {
				t.Errorf("%s includes %q, which doesn't exist", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk content: %v", err)
	}
	var examples int
	err = filepath.Walk("examples", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		examples++
		if !shown[path] {
			t.Errorf("%s isn't shown on any page", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	if examples == 0 {
		t.Fatal("no examples under examples/, so this asserted nothing")
	}
}
