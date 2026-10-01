package ns

import "strconv"

// A Value is a scalar fact value. A fact carries a string and, when numeric, a number, so a bound
// term keeps both: string equality and numeric comparison both work with no re-parse.
type Value struct {
	S   string
	Num *float64
	// Absent marks a field the source did not state AT ALL, which is different from an empty string
	// and different from zero. A datasheet row stating only a maximum leaves its minimum absent.
	//
	// It is a field rather than something inferred from a nil Num because the two are not the same
	// question: a non-numeric string also has a nil Num, and conflating them means the engine
	// RECONSTRUCTS absence from a coincidence instead of representing it. Before it was
	// representable, an absent field bound to the empty string and ordering fell through to string
	// order, where "" precedes everything and so "passed" every upper-bound test, including against a
	// negative threshold.
	Absent bool
	// BaseUnit is the SI BASE symbol this value's number is expressed in ("V", "A", UnitOhm), or ""
	// for a dimensionless value or a non-numeric one.
	//
	// NEVER A PREFIXED SPELLING. Scale normalization is the host's job and happens before a value
	// reaches the engine, so a millivolt reading is already volts by the time it reaches a query.
	// This layer checks DIMENSION (is this volts or amps) and never converts SCALE. Naming the field
	// for that invariant is deliberate: a projector that set "mV" here would make a
	// volts-against-millivolts comparison REFUSE rather than convert, which is a fresh silent wrong
	// answer wearing the fix's clothes. A host should test that it never publishes a prefixed unit.
	//
	// Distinct from the `param.unit` RELATION, which reports what the vendor PRINTED ("mV") for a
	// human checking a citation. This says what the number IS in, for a machine comparing it.
	BaseUnit string
}

// ftoa is the canonical string form of a number, the S that N and NU give a numeric Value.
func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
