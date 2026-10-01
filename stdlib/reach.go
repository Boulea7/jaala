package stdlib

import (
	"context"
	"errors"
	"strconv"

	"github.com/panyam/jaala/ns"
)

// A Walk visits what is reachable from start, out along edges (out true) or back against them, calling
// visit once per node reached with its path from start: the citations of the steps taken, in walk
// order. The host decides what an edge is (which relation, which kinds, ties, what not to walk
// through); extra holds the walk's other arguments, such as the edge kinds to follow. A Walk should
// stop and return ctx.Err() when ctx ends, and return visit's error as is.
type Walk func(ctx context.Context, start ns.Value, out bool, extra []ns.Value, visit func(node ns.Value, path []string) error) error

// Reach builds a transitive-closure predicate, reach(?from, ?to, extra...), from a host's walk, so
// each host writes only its edge semantics and gets the rest the same way:
//
//   - It runs from whichever end a body binds: out from ?from when that is bound, back from ?to when
//     only ?to is. Its Modes say so, so the planner schedules it from the bound end and a body that
//     binds neither is refused at validation, naming the predicate.
//   - With both ends bound it walks out from ?from and stops at ?to.
//   - Each answer cites its path's steps in order from ?from to ?to (a backward walk's path is
//     reversed to read that way), which a Witness keeps in that order.
//
// The extra arguments must be bound; extra gives their types. Arguments are labelled from, to and
// argN; set Labels, Types and Doc on the result to name them for a host's catalogue.
func Reach(walk Walk, extra ...ns.ArgType) ns.Builtin {
	n := 2 + len(extra)
	out, back := make([]bool, n), make([]bool, n)
	out[0], back[1] = true, true
	labels := []string{"from", "to"}
	for i := 2; i < n; i++ {
		out[i], back[i] = true, true
		labels = append(labels, "arg"+strconv.Itoa(i))
	}
	return ns.Builtin{
		Arity:  n,
		Modes:  [][]bool{out, back},
		Labels: labels,
		Types:  append([]ns.ArgType{{}, {}}, extra...),
		Doc:    "reports what is reachable from ?from, or reaches ?to, with the path as its citations",
		Gen: func(ctx context.Context, _ ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
			ext := make([]ns.Value, 0, len(args)-2)
			for _, a := range args[2:] {
				ext = append(ext, a.Value)
			}
			from, to := args[0], args[1]
			row := func(f, t ns.Value) []ns.Value { return append([]ns.Value{f, t}, ext...) }
			if from.Bound {
				err := walk(ctx, from.Value, true, ext, func(node ns.Value, path []string) error {
					if to.Bound && !sameValue(node, to.Value) {
						return nil
					}
					if err := emit(row(from.Value, node), path); err != nil {
						return err
					}
					if to.Bound {
						return errFound
					}
					return nil
				})
				if errors.Is(err, errFound) {
					return nil
				}
				return err
			}
			return walk(ctx, to.Value, false, ext, func(node ns.Value, path []string) error {
				return emit(row(node, to.Value), reversed(path))
			})
		},
	}
}

// errFound stops a walk once a bound target is reached.
var errFound = errors.New("stdlib: reach found its target")

func reversed(path []string) []string {
	out := make([]string, len(path))
	for i, s := range path {
		out[len(path)-1-i] = s
	}
	return out
}

// sameValue is value equality as the engine unifies it: an absent value equals only an absent one,
// numbers compare by value, and anything else by its string.
func sameValue(a, b ns.Value) bool {
	if a.Absent || b.Absent {
		return a.Absent && b.Absent
	}
	if a.Num != nil && b.Num != nil {
		return *a.Num == *b.Num
	}
	return a.S == b.S
}
