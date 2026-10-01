package ns

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func moduleErr(t *testing.T, err error) *ModuleError {
	t.Helper()
	var me *ModuleError
	if !errors.As(err, &me) {
		t.Fatalf("err = %v (%T), want a *ModuleError", err, err)
	}
	return me
}

func TestAModulesOriginReachesLookup(t *testing.T) {
	v, _ := newTestVocabulary(t)
	if err := v.AddModule("net", "stub", "has_tp/1", "lib/net/tp.dl"); err != nil {
		t.Fatal(err)
	}
	if got := v.Modules()[0].Origin; got != "lib/net/tp.dl" {
		t.Errorf("Modules()[0].Origin = %q", got)
	}
	if e, err := v.Lookup("net.has_tp"); err != nil || e.Origin != "lib/net/tp.dl" {
		t.Errorf("Lookup = %+v, %v; want origin lib/net/tp.dl", e, err)
	}
}

// Two modules share net; only the second is refused, and the error says which one and where it came
// from, with its message as it always read.
func TestAddModuleNamesTheModuleItRefuses(t *testing.T) {
	v, _ := newTestVocabulary(t)
	if err := v.AddModule("net", "stub", "has_tp/1", "shipped/net.dl"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path, lang, text, want string
	}{
		{"net", "stub", "pin_count/2", `"net.pin_count" is defined twice`},
		{"net", "stub", "bad", `bad stub member "bad"`},
		{"net", "prolog", "x/1", `written in "prolog"`},
		{"a..b", "stub", "x/1", `path "a..b" has an empty segment`},
	} {
		err := v.AddModule(c.path, c.lang, c.text, "project/lib/net.dl")
		me := moduleErr(t, err)
		if me.Module != 1 || me.Path != c.path || me.Origin != "project/lib/net.dl" || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %+v, want module 1 at %q from project/lib/net.dl, message with %q", c.text, me, c.path, c.want)
		}
		if err.Error() != me.Err.Error() {
			t.Errorf("%s: Error() = %q, want the cause's message unchanged", c.text, err)
		}
	}
}

func TestAddModulesFSByDirectory(t *testing.T) {
	v, _ := newTestVocabulary(t)
	fsys := fstest.MapFS{
		"lib/go/test.dl":     {Data: []byte("test/1")},
		"lib/go/covers.dl":   {Data: []byte("covers/2")},
		"lib/http/route.dl":  {Data: []byte("route/2")},
		"lib/top.dl":         {Data: []byte("top/1")},
		"lib/go/README.md":   {Data: []byte("not a module")},
		"other/ignored.dl":   {Data: []byte("ignored/1")},
		"lib/go/deep/sub.dl": {Data: []byte("sub/1")},
	}
	if err := v.AddModulesFS(fsys, "lib", "stub", ByDirectory(".dl")); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range v.Modules() {
		got = append(got, m.Origin+"->"+m.Path)
	}
	want := []string{
		"lib/go/covers.dl->go", "lib/go/deep/sub.dl->go.deep", "lib/go/test.dl->go",
		"lib/http/route.dl->http", "lib/top.dl->",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("modules, in walk order:\n got  %v\n want %v", got, want)
	}
	for _, p := range []string{"go.test", "go.covers", "go.deep.sub", "http.route", "top"} {
		if !v.Has(p) {
			t.Errorf("%s not registered", p)
		}
	}
}

func TestAddModulesFSByFileName(t *testing.T) {
	v, _ := newTestVocabulary(t)
	fsys := fstest.MapFS{
		"lib/net.dl":      {Data: []byte("has_tp/1")},
		"lib/house.dl":    {Data: []byte("room/1")},
		"lib/sub/a.b.dl":  {Data: []byte("c/1")},
		"lib/notes.txt":   {Data: []byte("skipped")},
		"lib/net.dl.orig": {Data: []byte("skipped")},
	}
	if err := v.AddModulesFS(fsys, "lib", "stub", ByFileName(".dl")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"net.has_tp", "house.room", "a.b.c"} {
		if !v.Has(p) {
			t.Errorf("%s not registered", p)
		}
	}
	if n := len(v.Modules()); n != 3 {
		t.Errorf("%d modules, want 3", n)
	}
}

// A refused file leaves the vocabulary as it was, and the error names the file.
func TestAddModulesFSIsAllOrNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		fsys fstest.MapFS
		file string
	}{
		{"a file the language refuses", fstest.MapFS{"lib/a/ok.dl": {Data: []byte("fine/1")}, "lib/b/broken.dl": {Data: []byte("bad")}}, "lib/b/broken.dl"},
		{"a directory no path can spell", fstest.MapFS{"lib/a/ok.dl": {Data: []byte("fine/1")}, "lib/b c/x.dl": {Data: []byte("x/1")}}, "lib/b c/x.dl"},
	} {
		v, _ := newTestVocabulary(t)
		before := len(v.Modules())
		err := v.AddModulesFS(c.fsys, "lib", "stub", ByDirectory(".dl"))
		if me := moduleErr(t, err); me.Origin != c.file {
			t.Errorf("%s: error from %q, want %q", c.name, me.Origin, c.file)
		}
		if len(v.Modules()) != before || v.Has("a.fine") {
			t.Errorf("%s: a refused tree left %d modules and a.fine=%v behind", c.name, len(v.Modules())-before, v.Has("a.fine"))
		}
	}
}
