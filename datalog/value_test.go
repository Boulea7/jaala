package datalog

import (
	"fmt"
	"math"
	"testing"

	"github.com/panyam/jaala/ns"
)

// v is a present scalar; absentV is a field the source did not state. Both are spelled out here
// because the whole point of this change is that they are different things.
func numV(x float64, unit string) ns.Value { return ns.Value{S: ftoa(x), Num: &x, BaseUnit: unit} }
func absentV() ns.Value                    { return ns.Value{Absent: true} }

func cmpOf(t *testing.T, l ns.Value, op string, r ns.Value) bool {
	t.Helper()
	got, err := evalCompare(Compare{Left: Term{Const: &l}, Op: op, Right: Term{Const: &r}}, newBinding())
	if err != nil {
		t.Fatalf("evalCompare(%v %s %v): %v", l, op, r, err)
	}
	return got
}

// TestAbsentIsNotEmptyString is the defect this representation exists to remove. An absent field used
// to be Value{S: ""}, indistinguishable from a field genuinely stated as the empty string, so nothing
// downstream could tell "the datasheet states no minimum" from "the minimum is blank".
func TestAbsentIsNotEmptyString(t *testing.T) {
	if valueEq(absentV(), ns.Value{S: ""}) {
		t.Error("an absent field must not unify with a stated empty string")
	}
	if !valueEq(absentV(), absentV()) {
		t.Error("two absent fields must unify; see the SQL deviation below")
	}
	if !valueEq(ns.Value{S: ""}, ns.Value{S: ""}) {
		t.Error("two stated empty strings must still unify")
	}
}

// TestAbsentEqualsAbsentDeviatesFromSQL pins a decision rather than a behaviour, so it must fail
// loudly if someone "fixes" it toward SQL. SQL says NULL = NULL is UNKNOWN. This engine says two
// unstated bounds ARE the same answer to "what does this row state", because full three-valued logic
// would have to thread UNKNOWN through negation, aggregation and the index for a reading no engineer
// running a search has asked for.
func TestAbsentEqualsAbsentDeviatesFromSQL(t *testing.T) {
	if !cmpOf(t, absentV(), "=", absentV()) {
		t.Error("absent = absent must be TRUE here (deliberately not SQL's UNKNOWN)")
	}
	if cmpOf(t, absentV(), "!=", absentV()) {
		t.Error("absent != absent must be false")
	}
}

// TestCompareRefusesToOrderAbsentAgainstPresent is inherited from the previous PR and must keep
// passing under the new representation: it is the acceptance test for the whole change. The
// minus-two case is what proves the old behaviour was string ordering rather than any semantics.
func TestCompareRefusesToOrderAbsentAgainstPresent(t *testing.T) {
	for _, op := range []string{"<", "<=", ">", ">="} {
		for _, c := range []struct {
			name string
			l, r ns.Value
		}{
			{"absent on the left", absentV(), numV(5, "")},
			{"absent on the right", numV(5, ""), absentV()},
			{"absent against a negative threshold", absentV(), numV(-2, "")},
			{"absent against absent", absentV(), absentV()},
		} {
			if cmpOf(t, c.l, op, c.r) {
				t.Errorf("%s: %q must not match; an unstated value is not orderable", c.name, op)
			}
		}
	}
}

// TestCompareRefusesToOrderUnlikeDimensions: volts are neither smaller nor larger than amps. Scale is
// NOT this layer's problem and cannot reach it, so this is only ever "V" against "A" and never "mV"
// against "V" (param.InBaseUnit normalizes far upstream, C24).
func TestCompareRefusesToOrderUnlikeDimensions(t *testing.T) {
	volts, amps := numV(5, "V"), numV(3, "A")
	for _, op := range []string{"<", "<=", ">", ">="} {
		if cmpOf(t, volts, op, amps) {
			t.Errorf("%q must not match across unlike dimensions", op)
		}
	}
	// Same dimension still compares normally.
	if !cmpOf(t, numV(3, "V"), "<", numV(5, "V")) {
		t.Error("3V < 5V must still hold")
	}
}

// TestBareLiteralIsDimensionPolymorphic is the compatibility guarantee. A query constant has no way
// to say "5 volts", so a strict dimension match would refuse every threshold query ever written. An
// empty BaseUnit is polymorphic rather than a dimension of its own.
func TestBareLiteralIsDimensionPolymorphic(t *testing.T) {
	if !cmpOf(t, numV(3, "V"), "<", numV(5, "")) {
		t.Error("a volts value against a bare literal must still compare; literals cannot state a unit")
	}
	if !cmpOf(t, numV(3, ""), "<", numV(5, "A")) {
		t.Error("a bare literal against an amps value must still compare")
	}
}

// TestEqualityIsNotDimensionChecked: the same values also unify implicitly when a variable repeats
// across atoms, and unification is identity rather than physics. Making the explicit operator
// dimension-aware while unification is not would be incoherent, and making both would break joins and
// the fact index, which bucket by string value.
func TestEqualityIsNotDimensionChecked(t *testing.T) {
	if !cmpOf(t, numV(5, "V"), "=", numV(5, "A")) {
		t.Error("equality is deliberately not dimension-checked; see the evalCompare comment")
	}
}

// TestCompareStillOrdersTwoStrings and equality across kinds are unchanged by the absence work.
func TestCompareStillOrdersTwoStrings(t *testing.T) {
	if !cmpOf(t, ns.Value{S: "ALPHA"}, "<", ns.Value{S: "BETA"}) {
		t.Error(`"ALPHA" < "BETA" must still hold; string ordering is a feature`)
	}
}

func TestCompareStillEqualsAcrossKinds(t *testing.T) {
	if cmpOf(t, ns.Value{S: "ALPHA"}, "=", numV(5, "")) {
		t.Error("a word does not equal a number")
	}
	if !cmpOf(t, ns.Value{S: "ALPHA"}, "!=", numV(5, "")) {
		t.Error("a word is not-equal to a number")
	}
}

// TestAbsentDoesNotCollideInIndex: the fact index buckets by string key, and an absent value's key
// used to be "" — the same bucket a stated empty string files under. A probe for one would find the
// other, and only the exact comparison downstream would reject it.
func TestAbsentDoesNotCollideInIndex(t *testing.T) {
	// One key each is now true by construction: a value has exactly one key (valueKey, #162).
	if a, e := valueKey(absentV()), valueKey(ns.Value{S: ""}); a == e {
		t.Errorf("absent and empty-string share the index bucket %q", a)
	}
}

// -0 equals 0, so the index files them under a shared key: a probe for one finds the other, as a scan
// does (#89, found by FuzzEval, where a rule head's -0 never met the goal's 0 once the relation was
// big enough to index). control: Unindexed finds it by scanning.
func TestNegativeZeroSharesAnIndexKeyWithZero(t *testing.T) {
	if neg, zero := valueKey(ns.N(math.Copysign(0, -1))), valueKey(ns.N(0)); neg != zero {
		t.Errorf("valueKey(-0) = %q and valueKey(0) = %q differ", neg, zero)
	}
	src := ns.NewMemSource().Declare("w", "node", "n")
	for i := 0; i < 2*IndexMinTuples; i++ {
		src.Add("w", ns.Tuple{Vals: []ns.Value{ns.S(fmt.Sprintf("v%d", i)), ns.N(float64(i))}})
	}
	q := mustParse(t, `w(?n, -0) => ?n`)
	b := baseFor(std(src))
	for _, base := range []*Base{b.Unindexed(), b} {
		rows, err := (SemiNaive{}).Eval(bg, q, base)
		if err != nil || col(rows, "n") != "v0" {
			t.Errorf("w(?n, -0) = %v, %v; want v0 (unindexed %v)", col(rows, "n"), err, base.noIndex)
		}
	}
}
