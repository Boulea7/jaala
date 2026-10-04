package host_test

import (
	"fmt"

	"github.com/panyam/jaala/ns"
)

// source is the facts a host serves. A MemSource is the simplest Source. You
// declare each relation with its column labels, then add tuples, each with
// the citations an answer that uses it will show. A real host implements
// ns.Source over its own data instead.
func source() *ns.MemSource {
	src := ns.NewMemSource().Declare("imports", "from", "to")
	edges := [][2]string{
		{"app", "api"}, {"api", "auth"}, {"api", "log"},
		{"auth", "log"}, {"log", "util"},
	}
	for i, e := range edges {
		src.Add("imports", ns.Tuple{
			Vals:  []ns.Value{ns.S(e[0]), ns.S(e[1])},
			Cites: []string{fmt.Sprintf("imports.txt:%d", i+1)},
		})
	}
	return src
}
