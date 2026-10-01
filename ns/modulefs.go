package ns

import (
	"io/fs"
	"path"
	"strings"
)

// A Layout maps a module file, by its path relative to the root AddModulesFS walks, to the module
// path it registers at. A file it reports false for is not a module and is skipped.
type Layout func(file string) (module string, ok bool)

// ByDirectory is the layout where a directory is a module and each file in it is one module text:
// a/b/x.dl registers at module a.b, and x.dl at the root. Files without ext are skipped.
func ByDirectory(ext string) Layout {
	return func(file string) (string, bool) {
		if !strings.HasSuffix(file, ext) {
			return "", false
		}
		dir := path.Dir(file)
		if dir == "." {
			return "", true
		}
		return strings.ReplaceAll(dir, "/", "."), true
	}
}

// ByFileName is the layout where a file's name is its module path: a.b.dl registers at module a.b,
// wherever it sits under the root. Files without ext are skipped.
func ByFileName(ext string) Layout {
	return func(file string) (string, bool) {
		if !strings.HasSuffix(file, ext) {
			return "", false
		}
		return strings.TrimSuffix(path.Base(file), ext), true
	}
}

// AddModulesFS registers every module file under root in fsys, written in lang, at the module path
// layout gives it, one AddModule per file in lexical order, each with its file (root/file) as its
// origin. So a member knows the file defining it (Entry.Origin), and a refusal names the file
// (ModuleError.Origin).
//
// It is all or nothing: if any file is refused, or a file's module path cannot be spelled, the
// vocabulary is left exactly as it was. Like AddModule, it does not check what the modules read;
// Check does, once everything is registered.
func (v *Vocabulary) AddModulesFS(fsys fs.FS, root, lang string, layout Layout) error {
	trial := v.Clone()
	err := fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := name
		if root != "." {
			rel = strings.TrimPrefix(name, root+"/")
		}
		module, ok := layout(rel)
		if !ok {
			return nil
		}
		text, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		return trial.AddModule(module, lang, string(text), name) // refuses an unspellable module path too
	})
	if err != nil {
		return err
	}
	*v = *trial
	return nil
}
