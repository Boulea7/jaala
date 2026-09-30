package datalog

import (
	"strings"
	"testing"
)

// The two shapes that ship today, each with its disequality removed, are the cases this exists for.
// Both are real rule bodies: the interface-presence rule and the pull-up rule.
func TestNonInjectiveRulesFlagsTheShippedShapes(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want []string
	}{{
		// Presence: two matching signals mean the interface is in use. Without the disequality ONE
		// matching signal satisfies both atoms, so the interface reports in use on half the evidence.
		name: "presence without the disequality",
		text: `has_signal(?x) :- component-on-net(?x,?n); in_use(?x) :- has_signal(?x), has_signal(?y); in_use(?z) => ?z`,
		want: []string{"in_use"},
	}, {
		name: "presence with the disequality",
		text: `has_signal(?x) :- component-on-net(?x,?n); in_use(?x) :- has_signal(?x), has_signal(?y), ?x != ?y; in_use(?z) => ?z`,
		want: nil,
	}, {
		// Pull-up: a resistor from the signal net to a rail. Without the disequality the "rail" may
		// be the signal net itself, so a resistor with both ends on one net reads as a pull-up.
		name: "pull-up without the disequality",
		text: `pulled(?n) :- component-on-net(?pu,?n), component.class(?pu,"resistor"), component-on-net(?pu,?rail); pulled(?x) => ?x`,
		want: []string{"pulled"},
	}, {
		name: "pull-up with the disequality",
		text: `pulled(?n) :- component-on-net(?pu,?n), component.class(?pu,"resistor"), component-on-net(?pu,?rail), ?rail != ?n; pulled(?x) => ?x`,
		want: nil,
	}} {
		t.Run(c.name, func(t *testing.T) {
			got := NonInjectiveRules(mustParse(t, c.text))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("NonInjectiveRules = %v, want %v", got, c.want)
			}
		})
	}
}

// The cases that must stay quiet, each for a different reason. A lint that fires on correct rules
// gets switched off, and these are all real shapes the catalog either ships or is about to.
func TestNonInjectiveRulesStaysQuiet(t *testing.T) {
	for _, c := range []struct{ name, text string }{{
		// Two POSITIONS of one atom, not two atoms. reaches is reflexive (from == to at distance
		// zero), so a clamp on the net itself is a genuine answer and a disequality would break it.
		// This is the shape the reach-migration rules take.
		name: "reflexive generator across two positions of one atom",
		text: `clamped(?n) :- reaches(?n,?rn,?h), ?h <= 2, component-on-net(?t,?rn), component.class(?t,"tvs"); clamped(?x) => ?x`,
	}, {
		// The shipped termination rule. suffix appears twice with ?h and ?l at position 0 and nothing
		// separating them, but the two atoms carry DIFFERENT constants at position 1, so the author
		// has already said these are different questions. Flagging it would be noise on a correct rule.
		name: "same relation, different constants",
		text: `terminated(?h) :- component-on-net(?r,?h), suffix(?h,"_CANH"), reaches(?h,?l), suffix(?l,"_CANL"); terminated(?x) => ?x`,
	}, {
		// The same variable twice is one node by construction, which is what the author wrote.
		name: "same variable in both atoms",
		text: `both(?n) :- component-on-net(?p,?n), component-on-net(?p,?n); both(?x) => ?x`,
	}, {
		// Two atoms differing in MORE than one position are independent uses of the relation, not one
		// role filled twice. Requiring them distinct would be wrong on its face: the same component
		// legitimately sits on two nets, so `component-on-net(?a,?n), component-on-net(?c,?m)` must
		// not demand ?a != ?c. Only the identical-except-here shape says "two of the same thing".
		name: "two atoms differing in several positions",
		text: `pair(?a) :- component-on-net(?a,?n), component-on-net(?c,?m); pair(?x) => ?x`,
	}, {
		name: "a relation mentioned once cannot accuse itself",
		text: `single(?a) :- component-on-net(?a,?n), component.class(?a,"resistor"); single(?x) => ?x`,
	}} {
		t.Run(c.name, func(t *testing.T) {
			if got := NonInjectiveRules(mustParse(t, c.text)); len(got) > 0 {
				t.Errorf("NonInjectiveRules = %v, want none: %s", got, c.text)
			}
		})
	}
}

// A disequality separates a pair regardless of which way round it is written, since ?a != ?b and
// ?b != ?a mean the same thing and an author writes whichever reads better.
func TestNonInjectiveRulesAcceptsEitherOrderOfDisequality(t *testing.T) {
	for _, text := range []string{
		`p(?n) :- component-on-net(?pu,?n), component-on-net(?pu,?rail), ?rail != ?n; p(?x) => ?x`,
		`p(?n) :- component-on-net(?pu,?n), component-on-net(?pu,?rail), ?n != ?rail; p(?x) => ?x`,
	} {
		if got := NonInjectiveRules(mustParse(t, text)); len(got) > 0 {
			t.Errorf("NonInjectiveRules = %v, want none: %s", got, text)
		}
	}
}

// Only != separates. An equality or an ordering comparison between the same two variables says
// something else entirely, and treating it as separation would let the real bug through.
func TestNonInjectiveRulesIgnoresOtherComparisons(t *testing.T) {
	for _, op := range []string{"=", "<", "<=", ">", ">="} {
		text := `p(?n) :- component-on-net(?pu,?n), component-on-net(?pu,?rail), ?rail ` + op + ` ?n; p(?x) => ?x`
		if got := NonInjectiveRules(mustParse(t, text)); len(got) == 0 {
			t.Errorf("op %q was treated as separating the pair; only != separates", op)
		}
	}
}
