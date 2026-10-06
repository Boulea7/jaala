package datalog

import "github.com/panyam/jaala/ns"

// What a Source serves is normalized as the Base reads it, so by the time a query runs every value in
// a typed argument is that type and every plain number is written canonically (#162). A pin a Source
// stored as the number 3 in an entity argument reads as the text "3", as a query's 3 there does
// (coerceConstants); text in a number argument reads as its number when it is one; 1.50 reads as 1.5
// wherever it is. Text that says more than its number, such as 3.3V, keeps its text. Equality can then
// be strict, a number never equalling text, without a pin stored the wrong way going unmatched.

// normalizeTuples returns tuples with each value read as its argument's type (coerceValue) and each
// plain number written canonically (canonicalNumber). A value the type can't read stays as the Source
// gave it. The Source's slices are never written: a tuple that changes is copied, and when none does
// tuples itself comes back.
func normalizeTuples(tuples []ns.Tuple, types []ns.ArgType) []ns.Tuple {
	var out []ns.Tuple
	for i, t := range tuples {
		var vals []ns.Value
		for j, v := range t.Vals {
			n := v
			if j < len(types) {
				if c, ok := coerceValue(v, types[j]); ok {
					n = c
				}
			}
			n = canonicalNumber(n)
			if sameValue(n, v) {
				continue
			}
			if vals == nil {
				vals = append([]ns.Value(nil), t.Vals...)
			}
			vals[j] = n
		}
		if vals == nil {
			continue
		}
		if out == nil {
			out = append([]ns.Tuple(nil), tuples...)
		}
		out[i] = ns.Tuple{Vals: vals, Cites: t.Cites}
	}
	if out == nil {
		return tuples
	}
	return out
}

// sameValue reports whether a and b are the same value written the same way.
func sameValue(a, b ns.Value) bool {
	if a.Absent != b.Absent || a.S != b.S || a.BaseUnit != b.BaseUnit || (a.Num == nil) != (b.Num == nil) {
		return false
	}
	return a.Num == nil || *a.Num == *b.Num || (*a.Num != *a.Num && *b.Num != *b.Num)
}

// argTypesOf is rel's declared argument types, nil when it declares none.
func (b *Base) argTypesOf(rel string) []ns.ArgType {
	s, _ := b.schemaOf(rel)
	return s.Types
}
