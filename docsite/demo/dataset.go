package demo

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// A Dataset is a fixture as the docs show it: what it's about, and each relation with its columns
// and facts, in the order the fixture first states them.
type Dataset struct {
	Name        string
	Description string
	Relations   []DatasetRelation
	Text        string
}

// DatasetRelation is one relation of a dataset.
type DatasetRelation struct {
	Name    string
	Columns []string
	Facts   []Fact
}

// Fixtures lists the fixtures by name, sorted.
func Fixtures() []string {
	entries, _ := fs.ReadDir(fixtures, "fixtures")
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".facts"); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// LoadDataset reads a fixture: its leading comment lines are the description, a "#: rel(a, b)" line
// names a relation's columns, and the rest are its facts (see ParseFacts).
func LoadDataset(name string) (Dataset, error) {
	text, err := Fixture(name)
	if err != nil {
		return Dataset{}, err
	}
	d := Dataset{Name: name, Text: text}
	var desc []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if c, ok := strings.CutPrefix(line, "#:"); ok {
			rel, cols := schemaOf(strings.TrimSpace(c))
			d.Relations = append(d.Relations, DatasetRelation{Name: rel, Columns: cols})
			continue
		}
		if t, ok := strings.CutPrefix(line, "#"); ok && len(d.Relations) == 0 {
			desc = append(desc, strings.TrimSpace(t))
		}
	}
	d.Description = strings.Join(desc, " ")
	facts, err := ParseFacts(text)
	if err != nil {
		return Dataset{}, err
	}
	for _, f := range facts {
		i := d.relation(f.Relation, len(f.Args))
		d.Relations[i].Facts = append(d.Relations[i].Facts, f)
	}
	return d, nil
}

func (d *Dataset) relation(name string, arity int) int {
	for i, r := range d.Relations {
		if r.Name == name {
			return i
		}
	}
	cols := make([]string, arity)
	for i := range cols {
		cols[i] = fmt.Sprintf("arg%d", i+1)
	}
	d.Relations = append(d.Relations, DatasetRelation{Name: name, Columns: cols})
	return len(d.Relations) - 1
}

// schemaOf reads "rel(a, b)" as a relation's name and column labels.
func schemaOf(s string) (string, []string) {
	open := strings.IndexByte(s, '(')
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return s, nil
	}
	var cols []string
	for _, c := range strings.Split(s[open+1:len(s)-1], ",") {
		cols = append(cols, strings.TrimSpace(c))
	}
	return strings.TrimSpace(s[:open]), cols
}

// labels are a fixture's declared column labels by relation, for Run to declare its relations with.
func labels(text string) map[string][]string {
	out := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		if c, ok := strings.CutPrefix(strings.TrimSpace(line), "#:"); ok {
			rel, cols := schemaOf(strings.TrimSpace(c))
			out[rel] = cols
		}
	}
	return out
}
