package stdlib

import "github.com/panyam/jaala/ns"

// Register registers everything this package provides: Strings and Absent. A host that wants other
// names, or only some of it, calls those itself.
func Register(v *ns.Vocabulary) error {
	for _, add := range []func(*ns.Vocabulary) error{Strings, Absent} {
		if err := add(v); err != nil {
			return err
		}
	}
	return nil
}
