package ns

import (
	"fmt"
	"sort"
	"strings"
)

// Unknown describes a name the vocabulary does not hold, for an error that follows "query: ". It names
// what is missing and adds a hint, so the most common newcomer error, a mistyped or half-remembered
// name, is answered with the vocabulary at the moment it is needed:
//
//	unknown relation "egde"; did you mean "edge"?
//	unknown module "nett" in "nett.pin_count"; did you mean "net.pin_count"?
//	"str" is a module, not a relation; it holds contains, glob, match, prefix, suffix
//
// A missing module is named as such because "unknown relation" would send the reader hunting a typo
// in the last segment when the first one is wrong.
func (v *Vocabulary) Unknown(name string) string {
	if !v.hasBase() {
		return fmt.Sprintf("unknown relation %q%s", name, v.Hint(name))
	}
	if v.IsModule(name) {
		return fmt.Sprintf("%q is a module, not a relation; it holds %s", name, strings.Join(v.children(name), ", "))
	}
	if mod, ok := v.missingModule(name); ok {
		return fmt.Sprintf("unknown module %q in %q%s", mod, name, v.Hint(name))
	}
	return fmt.Sprintf("unknown relation %q%s", name, v.Hint(name))
}

// Hint is the text after the name in an unknown-name error: `; did you mean "X"?` when a registered
// name is a plausible typo, an explanation when no relation is registered at all, and "" otherwise.
//
// When no base relation is registered it says that instead of guessing, because every name is
// unknown in that state and a typo hint would send the reader hunting a spelling mistake they did not
// make. A host that forgot to install its relations still runs; this is the one moment the omission
// is visible.
func (v *Vocabulary) Hint(name string) string {
	if !v.hasBase() {
		if v != nil && v.hinter != nil {
			return "; " + v.hinter.NoVocabularyHint()
		}
		return "; no relations are installed"
	}
	if s := v.suggest(name); s != "" {
		return fmt.Sprintf(`; did you mean %q?`, s)
	}
	return ""
}

// missingModule reports the module part of a dotted name when no module of that path exists.
func (v *Vocabulary) missingModule(name string) (string, bool) {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return "", false
	}
	mod := name[:i]
	return mod, !v.IsModule(mod)
}

// suggest returns the registered path a mistyped name most plausibly meant, or "".
//
// It searches where the name says to look. A name whose module exists is compared against that
// module's members only, so `net.pin_cout` finds `net.pin_count` and never a closer-spelled member
// of another module. A name whose module is missing is repaired at the module: the nearest existing
// module stands in for it, and the member is suggested if that module holds it, else the module is.
// A bare name that matches nothing at the root, but is the last segment of a member elsewhere, is
// suggested at that path, which is what a query written before a name moved into a module needs.
func (v *Vocabulary) suggest(name string) string {
	mod, leaf := "", name
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		mod, leaf = name[:i], name[i+1:]
	}
	if mod != "" && !v.IsModule(mod) {
		near := closest(mod, v.modulePaths())
		if near == "" {
			return ""
		}
		if _, ok := v.members[near+"."+leaf]; ok {
			return near + "." + leaf
		}
		return near
	}
	var cands []string
	for _, p := range v.candidates() {
		if parent, _ := splitPath(p); parent == mod {
			cands = append(cands, p)
		}
	}
	best := closestBy(leaf, cands, func(p string) string { _, l := splitPath(p); return l })
	if best == "" && mod == "" {
		for _, p := range v.candidates() {
			if _, l := splitPath(p); l == leaf && p != leaf {
				return p
			}
		}
	}
	return best
}

// candidates is every member path a suggestion may name: base relations in the Source's order, then
// predicates and derived relations, each sorted. On a tie the earlier candidate wins, which is why the
// Source controls the order. A private member is never registered at a path, so it is never offered.
func (v *Vocabulary) candidates() []string {
	out := append([]string(nil), v.baseOrder...)
	out = append(out, v.namesOf(kindPredicate)...)
	return append(out, v.namesOf(kindDerived)...)
}

// modulePaths is every module path, sorted.
func (v *Vocabulary) modulePaths() []string {
	out := make([]string, 0, len(v.modules))
	for m, n := range v.modules {
		if n > 0 {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// splitPath splits a path into its module and its last segment.
func splitPath(p string) (module, leaf string) {
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

// closest returns the candidate nearest to name when it is within a plausible-typo distance.
func closest(name string, cands []string) string {
	return closestBy(name, cands, func(s string) string { return s })
}

// closestBy is closest comparing name against key(candidate). The threshold scales with the name's
// length (a longer name tolerates more slips) with a floor of 2, so an unrelated token gets no
// misleading suggestion. On a tie the earlier candidate wins.
func closestBy(name string, cands []string, key func(string) string) string {
	best, bestDist := "", 0
	for _, c := range cands {
		d := levenshtein(name, key(c))
		if best == "" || d < bestDist {
			best, bestDist = c, d
		}
	}
	threshold := len(name)/4 + 1
	if threshold < 2 {
		threshold = 2
	}
	if best != "" && bestDist <= threshold {
		return best
	}
	return ""
}

// levenshtein is the edit distance (insert/delete/substitute) between two strings — the closeness
// measure suggestRelation ranks candidates by.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[len(rb)]
}

// DidYouMeanValue suggests the closest legal value for a rejected constant, as `, did you mean "X"?`,
// the value-level twin of Hint. A typo is the common case this whole check exists for, so naming the intended
// value is most of its worth; returns "" when nothing is close enough to be worth guessing.
func DidYouMeanValue(allowed []string, got string) string {
	best, bestDist := "", 0
	for _, want := range allowed {
		d := levenshtein(got, want)
		if best == "" || d < bestDist {
			best, bestDist = want, d
		}
	}
	threshold := len(got)/4 + 1
	if threshold < 2 {
		threshold = 2
	}
	if bestDist > threshold {
		best = ""
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(", did you mean %q?", best)
}

// DidYouMean returns the hint an engine appends to an unknown-name error: `; did you mean "X"?`
// when a registered path is a plausible typo of name, an explanation when v holds no base relation
// at all, and "" otherwise. It is v.Hint, for a host that validates names itself, such as one
// checking a rule that arrived over its own wire format, so its errors read the same as the engine's.
func DidYouMean(v *Vocabulary, name string) string { return v.Hint(name) }
