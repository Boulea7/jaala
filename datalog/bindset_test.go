package datalog

import (
	"strings"
	"testing"

	"github.com/panyam/jaala/ns"
)

func bindSet(v Var, vals ...string) Option {
	set := []ns.Value{}
	for _, s := range vals {
		set = append(set, ns.S(s))
	}
	return Bind(map[Var][]ns.Value{v: set})
}

// A variable bound to several values ranges over them (#132): the answer is the union, with each row's
// own citations and none from the binding, and an aggregate reduces across the whole set. Bound to
// none, it answers nothing, or the one row an aggregate gives over nothing. control: unbound, the same
// goals answer more, so each set below is narrowing.
func TestASetBoundVariableRangesOverItsValues(t *testing.T) {
	b := baseFor(std(graph()))
	for _, c := range []struct {
		goal string
		vals []string
		want string // col of ?b, or of the count
	}{
		{`edge(?a, ?b) => ?a, ?b`, []string{"a", "c"}, "b,d"},
		{`edge(?a, ?b) => ?a, ?b`, []string{"c", "a", "c"}, "b,d"},
		{`edge(?a, ?b) => ?a, ?b`, []string{"a", "zzz"}, "b"},
		{`edge(?a, ?b) => ?a, ?b`, nil, ""},
		{`edge(?a, ?b) => count(?b)`, []string{"a", "b"}, "2"},
		{`edge(?a, ?b) => count(?b)`, nil, "0"},
	} {
		rows, err := both(mustParse(t, c.goal), b, bindSet("a", c.vals...))
		key := "b"
		if strings.Contains(c.goal, "count") {
			key = "count(b)"
		}
		if err != nil || col(rows, key) != c.want {
			t.Errorf("%s with ?a in %v: %q, %v; want %q", c.goal, c.vals, col(rows, key), err, c.want)
		}
		unbound, _ := both(mustParse(t, c.goal), b)
		if col(unbound, key) == c.want {
			t.Errorf("control: %s answers %q unbound too, so it cannot show the set narrowing it", c.goal, c.want)
		}
	}
	rows, err := both(mustParse(t, `edge(?a, ?b) => ?a, ?b`), b, bindSet("a", "a", "c"))
	if err != nil || len(rows) != 2 || col(rows, "a") != "a,c" || strings.Join(rows[0].Cites, " ") != "edge:ab" || strings.Join(rows[1].Cites, " ") != "edge:cd" {
		t.Errorf("rows %v, %v; want a->b citing edge:ab and c->d citing edge:cd, with ?a its bound value", rows, err)
	}
	rows, err = both(mustParse(t, `edge(?a, ?b) => ?a, count(?b)`), b, bindSet("a", "a", "b", "x"))
	if err != nil || col(rows, "a") != "a,b" || col(rows, "count(b)") != "1,1" {
		t.Errorf("grouped by the set-bound column: %v, %v; want a and b, one each (x has no edge)", rows, err)
	}
}

// A derived relation the goal calls with a set-bound argument is evaluated only for the values bound:
// one query costs about what asking once per value does, and far less than deriving it in full. With a
// constant elsewhere in the goal, the planner would start from it and demand reach from the nodes edge
// gives, every node that reaches v250 (#132's planGoal). control: the full closure costs several times
// more, so the fixture can tell.
func TestASetBoundCallIsDemandedOnlyForItsValues(t *testing.T) {
	for _, c := range []struct {
		goal string
		rows int
	}{
		{`reach(?s, ?b) => ?s, ?b`, 399 + 199},
		{`reach(?s, ?b), edge(?b, "v250") => ?s, ?b`, 2},
	} {
		work := func(opts ...Option) (int64, int) {
			t.Helper()
			b := baseFor(std(reversedLine(400)))
			rows, err := SemiNaive{}.Eval(bg, mustParse(t, closure+c.goal), b, opts...)
			if err != nil {
				t.Fatal(err)
			}
			return b.Work(), len(rows)
		}
		set, n := work(bindSet("s", "v0", "v200"))
		if n != c.rows {
			t.Errorf("%s from v0 and v200: %d rows, want %d", c.goal, n, c.rows)
		}
		one, _ := work(bindSet("s", "v0"))
		other, _ := work(bindSet("s", "v200"))
		if float64(set) > 1.3*float64(one+other) {
			t.Errorf("%s: work %d for the set, %d+%d asking each alone; want about the same", c.goal, set, one, other)
		}
		full, _ := work()
		if full < 5*set {
			t.Errorf("control: %s in full did %d work, the set %d; want the full one several times more", c.goal, full, set)
		}
	}
	for _, ev := range []Evaluator{Naive{}, SemiNaive{WrittenOrder: true}} {
		b := baseFor(std(reversedLine(40)))
		rows, err := ev.Eval(bg, mustParse(t, closure+`reach(?s, ?b) => ?s, ?b`), b, bindSet("s", "v0", "v20"))
		if err != nil || len(rows) != 39+19 {
			t.Errorf("%T: %d rows, %v; want %d", ev, len(rows), err, 39+19)
		}
	}
}

// Each value of a set is checked as a single bound value is: read as its argument's type, refused
// when that type cannot read it, and refused when a closed Domain does not hold it, with the error
// naming the value. control: the set without the bad value answers.
func TestEachValueOfASetIsCheckedAsOne(t *testing.T) {
	src := typedNets()
	q := mustParse(t, `net.pin_count(?n, ?c) => ?n`)
	if _, err := both(q, baseFor(std(src)), bindSet("c", "10", "abc")); err == nil || err.Error() != `query: net.pin_count's "count" argument cannot be "abc" (it holds a number)` {
		t.Errorf("a bad member: %v, want the refusal naming \"abc\"", err)
	}
	answersAs(t, `net.pin_count(?n, ?c) => ?n`, `net.pin_count(?n, 10) => ?n`, bindSet("c", "10", "7"))
	// Text in a set compared with a number is read as a number, as a single bound value is: as text,
	// "9" sorts after "10" and VBUS would be missed.
	answersAs(t, `net.pin_count(?n, ?c), ?c > ?min => ?n`, `net.pin_count(?n, ?c), ?c > 9 => ?n`, bindSet("min", "9", "11"))
	answersAs(t, `component.net(?r, ?n) => ?n`, `component.net("3", ?n) => ?n`,
		Bind(map[Var][]ns.Value{"r": {ns.N(3), ns.S("U9")}}))

	role := ns.NewMemSource().DeclareSchema("role", ns.Schema{Arity: 2, Labels: []string{"node", "role"}, Types: []ns.ArgType{{}, {Domain: []string{"source", "sink"}}}})
	role.Add("role", ns.Tuple{Vals: []ns.Value{ns.S("a"), ns.S("sink")}})
	role.Add("role", ns.Tuple{Vals: []ns.Value{ns.S("b"), ns.Absent()}})
	rq := mustParse(t, `role(?n, ?r) => ?n`)
	if _, err := both(rq, baseFor(std(role)), bindSet("r", "sink", "sinkk")); err == nil || !strings.Contains(err.Error(), `role's "role" argument cannot be "sinkk"`) {
		t.Errorf("a misspelled member of a closed Domain: %v, want the Domain refusal naming it", err)
	}
	if rows, err := both(rq, baseFor(std(role)), Bind(map[Var][]ns.Value{"r": {ns.S("sink"), ns.Absent()}})); err != nil || col(rows, "n") != "a,b" {
		t.Errorf("control: sink and absent: %v, %v; want a,b", col(rows, "n"), err)
	}
}

// A set-bound variable anchors a negation, as a single bound one does, and ValidateBound needs only
// its name. Its values leave no node in a witness. control: the unanchored refusal still names ?m.
func TestASetBoundVariableAnchorsANegationAndLeavesNoWitness(t *testing.T) {
	v := std(graph())
	q := mustParse(t, `not node(?n) => ?n`)
	rows, err := both(q, baseFor(v), bindSet("n", "a", "zzz", "yyy"))
	if err != nil || col(rows, "n") != "yyy,zzz" {
		t.Errorf("not node(?n), ?n in a, zzz, yyy: %v, %v; want yyy,zzz", col(rows, "n"), err)
	}
	if err := ValidateBound(q, v, "n"); err != nil {
		t.Errorf("ValidateBound = %v, want nil", err)
	}
	if _, err := both(mustParse(t, `node(?n), not edge(?m, ?y) => ?n`), baseFor(v), bindSet("n", "a", "b")); err == nil || !strings.Contains(err.Error(), "(?m appears only inside") {
		t.Errorf("control: an unanchored negation = %v, want the refusal naming ?m", err)
	}

	r := mustParse(t, `r(?x, ?y) :- edge(?x, ?y); r(?x, ?z) :- edge(?x, ?y), r(?y, ?z); r(?s, ?t) => ?s, ?t`)
	for _, ev := range evaluators {
		rows, err := ev.Eval(bg, r, baseFor(v), bindSet("s", "a", "c"), Witnesses())
		if err != nil || len(rows) != 4 {
			t.Fatalf("%T: %d rows, %v; want 4", ev, len(rows), err)
		}
		for _, row := range rows {
			if len(row.Witness) != 1 || row.Witness[0].Relation != "r" {
				t.Errorf("%T: row %v has witness %v, want one node, for r", ev, row.Bind, row.Witness)
			}
		}
	}
}

// A generator whose mode needs its input bound runs from each value of a set-bound input, after the
// relation of values binds it. control: unbound, the mode is refused.
func TestASetBoundInputSatisfiesAGeneratorsMode(t *testing.T) {
	gv := std(line(6))
	walker(t, gv, [][]bool{{true, false}})
	q := mustParse(t, `walk(?s, ?e) => ?s, ?e`)
	rows, err := both(q, baseFor(gv), bindSet("s", "v1", "v3"))
	if err != nil || len(rows) != 6 || col(rows, "s") != "v1,v1,v1,v1,v3,v3" {
		t.Errorf("walk from v1 and v3: %v, %v; want v1's four steps and v3's two", rows, err)
	}
	if _, err := both(q, baseFor(gv)); err == nil {
		t.Errorf("control: walk with nothing bound ran, want the mode refusal")
	}
}
