package demo

import (
	"strings"
	"testing"
)

// A dataset reads its description from the leading comment, its columns from "#:" lines, and its
// facts grouped by relation in the order the fixture declares them. control: a relation with no
// "#:" line still gets a table, with numbered columns.
func TestLoadDataset(t *testing.T) {
	d, err := LoadDataset("deps")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.Description, "A small Go-style service") {
		t.Errorf("description %q", d.Description)
	}
	var got []string
	for _, r := range d.Relations {
		got = append(got, r.Name+"("+strings.Join(r.Columns, ",")+")")
	}
	if strings.Join(got, " ") != "package(name) imports(from,to) test(name,pkg) owner(pkg,team) loc(pkg,lines)" {
		t.Errorf("relations %v", got)
	}
	if n := len(d.Relations[1].Facts); n != 18 {
		t.Errorf("imports has %d facts, want 18", n)
	}
	if strings.Join(Fixtures(), ",") != "deps,graph" {
		t.Errorf("fixtures %v", Fixtures())
	}
	var x Dataset
	if i := x.relation("unlabelled", 2); strings.Join(x.Relations[i].Columns, ",") != "arg1,arg2" {
		t.Errorf("control: an undeclared relation's columns %v", x.Relations[i].Columns)
	}
}
