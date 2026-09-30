package datalog

import "fmt"

// didYouMean returns a `; did you mean "X"?` hint for a relation name the engine does not know,
// naming the closest known relation or predicate when one is a plausible typo, else "". It teaches
// the vocabulary at the exact moment a user gets it wrong, since the most common newcomer error is a
// mistyped or half-remembered relation name.
//
// When the Source serves NO relation it says that instead, because every name is unknown in that
// state and a typo hint would send the reader hunting a spelling mistake they did not make. A host
// that forgot to install its relations still runs; this is the one moment the omission is visible.
func (b *Base) didYouMean(name string) string {
	if !installed(b.src) {
		if h, ok := b.src.(NoVocabularyHinter); ok {
			return "; " + h.NoVocabularyHint()
		}
		return "; no relations are installed"
	}
	if s := b.suggestRelation(name); s != "" {
		return fmt.Sprintf(`; did you mean %q?`, s)
	}
	return ""
}

// suggestCandidates is every name a suggestion may name: the Source's relations in the order it
// prefers, then any predicate it did not already list, sorted.
func (b *Base) suggestCandidates() []string {
	rels := b.src.Relations()
	seen := make(map[string]bool, len(rels))
	out := make([]string, 0, len(rels))
	for _, r := range rels {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, p := range b.preds.Names() {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// suggestRelation returns the known name closest to `name` when it is within a plausible-typo
// distance, else "", so a genuinely unrelated token gets no misleading suggestion. The threshold
// scales with the name length (a longer name tolerates more slips) with a floor of 2. On a tie the
// earlier candidate wins, which is why the Source controls the order.
func (b *Base) suggestRelation(name string) string {
	best, bestDist := "", 0
	for _, r := range b.suggestCandidates() {
		d := levenshtein(name, r)
		if best == "" || d < bestDist {
			best, bestDist = r, d
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

// didYouMeanValue suggests the closest legal value for a rejected constant, the value-level twin of
// suggestRelation. A typo is the common case this whole check exists for, so naming the intended
// value is most of its worth; returns "" when nothing is close enough to be worth guessing.
func didYouMeanValue(allowed []string, got string) string {
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

// DidYouMean returns the hint the engine appends to an unknown-relation error: `; did you mean "X"?`
// when a known relation or predicate is a plausible typo of name, an explanation when src serves no
// relation at all, and "" otherwise. It is exported for a host that validates relation names itself,
// such as one checking a rule that arrived over its own wire format, so its errors read the same as
// the engine's.
func DidYouMean(src Source, preds *Predicates, name string) string {
	return (&Base{src: src, preds: preds}).didYouMean(name)
}
