package datalog

import "sort"

// A Tuple is one fact of a relation: its values in argument order, plus the citations that justify
// it. Base facts and derived facts share this shape, so a derived answer stays as verifiable as a
// looked-up one.
type Tuple struct {
	Vals  []Value
	Cites []string
}

// A Schema describes one relation a Source serves.
type Schema struct {
	// Arity is the number of arguments every tuple of the relation carries.
	Arity int
	// Labels names the arguments for error messages ("ref_des", "net"). Optional, and may be shorter
	// than Arity; an unlabelled argument is simply never named.
	Labels []string
	// Domains optionally closes an argument over a fixed vocabulary. A CONSTANT outside it is
	// rejected before evaluation rather than silently matching nothing, because an empty answer to
	// a question that was never valid reads as a fact about the data. A nil or empty entry, or an
	// index past the end, leaves that argument open. A domain is only enforced on a labelled
	// argument, since the error has to name what the argument is.
	Domains [][]string
}

// A Source is where the base (extensional) relations come from. The engine asks it for a relation's
// schema when it meets an atom, and for the relation's tuples the first time it needs them; it
// indexes and caches from there, so a Source need not.
//
// Tuples are treated as immutable for the life of a Base. A Source is read from one goroutine at a
// time per relation, but several Evals may share a Base concurrently.
type Source interface {
	// Schema reports whether the source serves rel, and its shape.
	Schema(rel string) (Schema, bool)
	// Tuples returns every fact of rel. Called at most once per relation per Base.
	Tuples(rel string) []Tuple
	// Relations lists every relation name the source serves, in the order a did-you-mean suggestion
	// should prefer on a tie. An EMPTY list means no vocabulary is installed at all, which the engine
	// treats differently from a vocabulary that lacks a name: it cannot call a relation unknown when
	// it knows of none, so it says so instead of guessing at a typo.
	Relations() []string
}

// NoVocabularyHinter is optionally implemented by a Source to explain an empty vocabulary in the
// host's terms, such as which import installs its relations. The text follows "; " in an unknown-
// relation error.
type NoVocabularyHinter interface {
	NoVocabularyHint() string
}

// MemSource is an in-memory Source, for tests and for hosts whose facts are already a handful of
// tables. Declare a relation, then Add its tuples.
type MemSource struct {
	schemas map[string]Schema
	tuples  map[string][]Tuple
	order   []string
}

// NewMemSource returns an empty MemSource.
func NewMemSource() *MemSource {
	return &MemSource{schemas: map[string]Schema{}, tuples: map[string][]Tuple{}}
}

// Declare adds a relation with the given argument labels; its arity is the number of labels.
// Declaring a name twice replaces its schema and keeps its tuples.
func (m *MemSource) Declare(rel string, labels ...string) *MemSource {
	if _, ok := m.schemas[rel]; !ok {
		m.order = append(m.order, rel)
	}
	m.schemas[rel] = Schema{Arity: len(labels), Labels: labels}
	return m
}

// DeclareSchema adds a relation with an explicit schema, for one that needs Domains.
func (m *MemSource) DeclareSchema(rel string, s Schema) *MemSource {
	if _, ok := m.schemas[rel]; !ok {
		m.order = append(m.order, rel)
	}
	m.schemas[rel] = s
	return m
}

// Add appends one tuple to a declared relation. It panics on an undeclared relation or a wrong
// arity, because either is a programming error in the host rather than a property of the data.
func (m *MemSource) Add(rel string, t Tuple) *MemSource {
	s, ok := m.schemas[rel]
	if !ok {
		panic("datalog: MemSource.Add to undeclared relation " + rel)
	}
	if len(t.Vals) != s.Arity {
		panic("datalog: MemSource.Add arity mismatch for " + rel)
	}
	m.tuples[rel] = append(m.tuples[rel], t)
	return m
}

// Schema implements Source.
func (m *MemSource) Schema(rel string) (Schema, bool) {
	s, ok := m.schemas[rel]
	return s, ok
}

// Tuples implements Source.
func (m *MemSource) Tuples(rel string) []Tuple { return m.tuples[rel] }

// Relations implements Source, listing names in sorted order.
func (m *MemSource) Relations() []string {
	out := append([]string(nil), m.order...)
	sort.Strings(out)
	return out
}

// S builds a string value.
func S(s string) Value { return Value{S: s} }

// N builds a numeric value; its string form is the canonical rendering of the number.
func N(f float64) Value { return Value{S: ftoa(f), Num: &f} }

// NU builds a numeric value carrying a base-unit tag.
func NU(f float64, unit string) Value { return Value{S: ftoa(f), Num: &f, BaseUnit: unit} }

// Absent is the value of an argument the source did not state at all.
func Absent() Value { return Value{Absent: true} }
