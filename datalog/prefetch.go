package datalog

import (
	"slices"

	"github.com/panyam/jaala/ns"
)

// A Source that batches lookups (ns.BatchLookupSource, #153) is asked once for every value the
// literals before a looked-up call bind it to, rather than once per value as solve reaches the call.
// solve is depth-first and never holds a body's bindings (#120), so the values aren't known until the
// body runs: prefetch solves the body up to the call, collects the distinct values not yet looked up,
// and asks for them batchSize at a time, and the solve that follows finds each in the Eval's cache. The
// prefix runs once more per looked-up call, work in memory spent to save a round trip per value.
// Anything the prefix misses is looked up as before, so the answer never depends on it.
//
// Only the planned SemiNaive prefetches (evalRun.batch): Naive is the reference, and the written order
// matches its citations, which a batch's tuple order could change.

// batchSize is the most values one batch carries, which bounds what prefetch holds at once.
const batchSize = 1024

// prefetch asks the Source for each looked-up call in lits, a body in the order solve runs it, in
// batches. A Volatile predicate stops it, since running the prefix again could answer differently.
func prefetch(b *Base, lits []Literal) error {
	if b.run == nil || !b.run.batch {
		return nil
	}
	bl, ok := b.looker.(ns.BatchLookupSource)
	if !ok {
		return nil
	}
	for i, lit := range lits {
		atom := lit.Pos
		if atom == nil {
			continue
		}
		if bi, ok := b.reg.Predicate(atom.Relation); ok {
			if bi.Volatile {
				return nil
			}
			continue
		}
		if _, base := b.schemaOf(atom.Relation); i == 0 || !base || !b.looksUp(atom.Relation) {
			continue // the first literal has only its constants bound, one lookup
		}
		if err := prefetchAt(b, bl, lits, i); err != nil {
			return err
		}
	}
	return nil
}

// prefetchAt solves lits[:i] and asks for the values it binds lits[i] to that the Eval hasn't looked up,
// grouped by the positions bound.
func prefetchAt(b *Base, bl ns.BatchLookupSource, lits []Literal, i int) error {
	atom := lits[i].Pos
	pending := map[patternMask][][]ns.Value{}
	asked := map[string]bool{}
	flush := func(mask patternMask) error {
		vals := pending[mask]
		delete(pending, mask)
		return b.run.explain.at(i, func() error { return b.lookupBatch(bl, atom, mask, vals) })
	}
	e := b.run.explain
	if e != nil {
		e.quiet = true
	}
	err := solve(lits[:i], 0, newBinding(), b, func(bnd *binding) error {
		mask, vals, any := boundArgs(atom.Args, bnd)
		if !any {
			return nil
		}
		key := lookupKey(atom.Relation, mask, vals)
		if _, done := b.run.looked[key]; done || asked[key] {
			return nil
		}
		asked[key] = true
		pending[mask] = append(pending[mask], vals)
		if len(pending[mask]) < batchSize {
			return nil
		}
		return flush(mask)
	})
	if e != nil {
		e.quiet = false
	}
	if err != nil {
		return err
	}
	for _, mask := range sortedMasks(pending) {
		if err := flush(mask); err != nil {
			return err
		}
	}
	return nil
}

// sortedMasks is pending's masks in order, so a body's batches go out in the same order every run.
func sortedMasks(pending map[patternMask][][]ns.Value) []patternMask {
	out := make([]patternMask, 0, len(pending))
	for m := range pending {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}
