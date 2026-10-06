package datalog

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/panyam/jaala/ns"
)

// A Source that implements ns.LookupSource answers a call with bound arguments from its own index
// (#126), so a relation the queries only probe is never read whole. What a lookup returns is kept on
// the Eval's run, not the Base: across queries the Base would otherwise grow into the whole relation
// one value at a time, holding it without ever having read it.

// looksUp reports whether a call to rel goes to the Source's Lookup: the Source answers lookups, the
// Base indexes (Unindexed scans, as the oracle an equivalence test compares against), and the Base
// doesn't already hold the relation whole, whose index answers without asking the Source.
func (b *Base) looksUp(rel string) bool {
	return b.looker != nil && !b.noIndex && b.run != nil && (b.edb == nil || !b.edb.holds(rel))
}

// lookup asks the Source for rel's facts matching atom's bound arguments, once per Eval for each
// pattern and values. ok is false when nothing is bound, so the caller reads the relation whole.
func (b *Base) lookup(atom *Atom, bnd *binding) (rows []ns.Tuple, ok bool, err error) {
	mask, vals, any := boundArgs(atom.Args, bnd)
	if !any {
		return nil, false, nil
	}
	r := b.run
	key := lookupKey(atom.Relation, mask, vals)
	if rows, hit := r.looked[key]; hit {
		if r.explain != nil {
			b.explainLookup(atom, mask, 0, true)
		}
		return rows, true, nil
	}
	if err := b.countWork(); err != nil {
		return nil, true, err
	}
	bound := make(map[int]ns.Value, len(vals))
	j := 0
	for i := range atom.Args {
		if i < maskWidth && mask&(1<<uint(i)) != 0 {
			bound[i] = vals[j]
			j++
		}
	}
	ctx := r.context()
	rows, err = b.looker.Lookup(ctx, atom.Relation, bound)
	if err != nil {
		if ctx.Err() != nil {
			return nil, true, fmt.Errorf("query: evaluation stopped looking up %s: %w", atom.Relation, ctx.Err())
		}
		return nil, true, fmt.Errorf("query: looking up %s: %w", atom.Relation, err)
	}
	rows = normalizeTuples(rows, b.argTypesOf(atom.Relation))
	if r.looked == nil {
		r.looked = map[string][]ns.Tuple{}
	}
	r.looked[key] = rows
	if r.explain != nil {
		b.explainLookup(atom, mask, len(rows), false)
	}
	return rows, true, nil
}

// lookupKey is a lookup's key in the Eval's cache: the relation, the bound positions, and their values'
// text. A number spelled two ways is looked up twice, which costs a call but never a match.
func lookupKey(rel string, mask patternMask, vals []ns.Value) string {
	var sb strings.Builder
	sb.WriteString(rel)
	sb.WriteByte(0)
	sb.WriteString(strconv.FormatUint(uint64(mask), 16))
	for _, v := range vals {
		k := keyText(v)
		sb.WriteByte(0)
		sb.WriteString(strconv.Itoa(len(k)))
		sb.WriteByte(':')
		sb.WriteString(k)
	}
	return sb.String()
}
