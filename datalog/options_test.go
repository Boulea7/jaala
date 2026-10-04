package datalog

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panyam/jaala/ns"
)

var evaluators = []Evaluator{Naive{}, SemiNaive{}, SemiNaive{WrittenOrder: true}}

// An Eval whose context ends stops promptly, wrapping the context's error, on every evaluator.
func TestEvalStopsWhenItsContextEnds(t *testing.T) {
	q := mustParse(t, leftReach+`reach(?a, ?b) => count(?b)`)
	for _, ev := range evaluators {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		start := time.Now()
		_, err := ev.Eval(ctx, q, baseFor(std(line(3000))))
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || !strings.HasPrefix(err.Error(), "query: evaluation stopped") {
			t.Errorf("%T: err = %v, want the deadline, wrapped", ev, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%T: stopped after %v, want promptly after its 30ms deadline", ev, d)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (SemiNaive{}).Eval(ctx, mustParse(t, `edge(?a, ?b)`), baseFor(std(graph()))); !errors.Is(err, context.Canceled) {
		t.Errorf("an already-cancelled context: err = %v, want context.Canceled", err)
	}
}

// The context reaches the host's generator, so a walk in progress stops too (Declaire's case: the
// time was inside graph.reach, not between calls).
func TestAGeneratorSeesTheContext(t *testing.T) {
	v := std(graph())
	if err := v.AddPredicate("slow", ns.Builtin{Arity: 1, Modes: [][]bool{{false}}, Gen: func(ctx context.Context, _ ns.Source, _ []ns.Arg, _ func([]ns.Value, []string) error) error {
		<-ctx.Done()
		return ctx.Err()
	}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := SemiNaive{}.Eval(ctx, mustParse(t, `slow(?x)`), baseFor(v))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Errorf("err = %v after %v, want the deadline promptly", err, time.Since(start))
	}
}

// ctxSource reads through TuplesContext, blocking until its context ends while blocked is set.
type ctxSource struct {
	*ns.MemSource
	mu      sync.Mutex
	blocked bool
	fail    error
	reads   int
}

func (s *ctxSource) TuplesContext(ctx context.Context, rel string) ([]ns.Tuple, error) {
	s.mu.Lock()
	s.reads++
	blocked, fail := s.blocked, s.fail
	s.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if fail != nil {
		return nil, fail
	}
	return s.MemSource.Tuples(rel), nil
}

// A cancellable Source's read stops with the query, and a read that did not finish is not kept: the
// next query reads the relation again under its own context. A read that fails is reported with the
// relation's name.
func TestASourceReadSeesTheContextAndIsNotCachedWhenItFails(t *testing.T) {
	src := &ctxSource{MemSource: graph(), blocked: true}
	b := MustBase(std(src.MemSource), src)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := (SemiNaive{}).Eval(ctx, mustParse(t, `edge(?a, ?b)`), b); !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "reading edge") {
		t.Fatalf("blocked read: err = %v, want the deadline while reading edge", err)
	}
	src.mu.Lock()
	src.blocked = false
	src.mu.Unlock()
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, `edge(?a, ?b)`), b)
	if err != nil || len(rows) != 3 || src.reads != 2 {
		t.Errorf("after the cancelled read: %d rows, %v, %d reads; want 3 rows from a second read", len(rows), err, src.reads)
	}
	failing := &ctxSource{MemSource: graph(), fail: errors.New("disk on fire")}
	_, err = SemiNaive{}.Eval(bg, mustParse(t, `edge(?a, ?b)`), MustBase(std(failing.MemSource), failing))
	if err == nil || err.Error() != "query: reading edge: disk on fire" {
		t.Errorf("failed read: err = %v", err)
	}
}

// A budget stops an Eval that works past it, with an error a host can tell from a program error; a
// generous one changes nothing. Each solution a generator emits counts.
func TestABudgetStopsAnEvalThatWorksPastIt(t *testing.T) {
	q := mustParse(t, leftReach+`reach(?a, ?b) => ?a, ?b`)
	for _, ev := range evaluators {
		_, err := ev.Eval(bg, q, baseFor(std(line(60))), Budget(500))
		var be *BudgetExceeded
		if !errors.As(err, &be) || be.Limit != 500 || be.Work != 501 {
			t.Errorf("%T: err = %v, want *BudgetExceeded at 501 of 500", ev, err)
		}
		want, err1 := ev.Eval(bg, q, baseFor(std(line(20))))
		got, err2 := ev.Eval(bg, q, baseFor(std(line(20))), Budget(1_000_000))
		if err1 != nil || err2 != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%T: a generous budget changed the answer: %v / %v", ev, err1, err2)
		}
	}
	v := std(graph())
	if err := v.AddPredicate("many", ns.Builtin{Arity: 1, Modes: [][]bool{{false}}, Gen: func(_ context.Context, _ ns.Source, _ []ns.Arg, emit func([]ns.Value, []string) error) error {
		for i := 0; i < 1000; i++ {
			if err := emit([]ns.Value{ns.N(float64(i))}, nil); err != nil {
				return err
			}
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	var be *BudgetExceeded
	if _, err := (SemiNaive{}).Eval(bg, mustParse(t, `many(?x)`), baseFor(v), Budget(100)); !errors.As(err, &be) {
		t.Errorf("a generator's 1000 rows under a budget of 100: err = %v, want *BudgetExceeded", err)
	}
}

// Budgets are per Eval: concurrent Evals on one Base do not spend each other's.
func TestConcurrentEvalsHaveTheirOwnBudgets(t *testing.T) {
	b := baseFor(std(line(80)))
	q := mustParse(t, leftReach+`reach(?a, ?b) => count(?b)`)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			limit := int64(100_000_000)
			if i%2 == 0 {
				limit = 50
			}
			_, errs[i] = SemiNaive{}.Eval(bg, q, b, Budget(limit))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		var be *BudgetExceeded
		tripped := errors.As(err, &be)
		if (i%2 == 0) != tripped || (tripped && be.Work != 51) {
			t.Errorf("Eval %d: err = %v; want only the small budgets to trip, each at its own 51", i, err)
		}
	}
}

// A bound variable is exactly a constant written in the goal: same rows, same work (demand and
// planning start from it), and it stays a column holding its value.
func TestABoundVariableIsAConstantInTheGoal(t *testing.T) {
	for _, ev := range evaluators {
		textual := baseFor(std(line(60)))
		want, err := ev.Eval(bg, mustParse(t, leftReach+`reach("v0", ?x) => ?x`), textual)
		if err != nil {
			t.Fatal(err)
		}
		bound := baseFor(std(line(60)))
		got, err := ev.Eval(bg, mustParse(t, leftReach+`reach(?s, ?x) => ?s, ?x`), bound, Bind(map[Var]ns.Value{"s": ns.S("v0")}))
		if err != nil || len(got) != len(want) {
			t.Fatalf("%T: %d rows, %v; want %d", ev, len(got), err, len(want))
		}
		for i := range got {
			if got[i].Bind["x"] != want[i].Bind["x"] || got[i].Bind["s"].S != "v0" {
				t.Errorf("%T row %d = %v, want %v with s=v0", ev, i, got[i].Bind, want[i].Bind)
				break
			}
		}
		if textual.Work() != bound.Work() {
			t.Errorf("%T: work %d bound, %d written; want the same (the binding is a constant)", ev, bound.Work(), textual.Work())
		}
	}
}

func TestBindingsAtTheEdges(t *testing.T) {
	b := baseFor(std(graph()))
	rows, err := SemiNaive{}.Eval(bg, mustParse(t, `edge(?a, ?b)`), b, Bind(map[Var]ns.Value{"a": ns.S("a"), "b": ns.S("b")}))
	if err != nil || len(rows) != 1 || rows[0].Bind["a"].S != "a" || rows[0].Bind["b"].S != "b" {
		t.Errorf("every column bound, the fact present: %v, %v; want one row a=a, b=b", rows, err)
	}
	if rows, _ := (SemiNaive{}).Eval(bg, mustParse(t, `edge(?a, ?b)`), b, Bind(map[Var]ns.Value{"a": ns.S("a"), "b": ns.S("d")})); len(rows) != 0 {
		t.Errorf("every column bound, the fact absent: %v, want no rows", rows)
	}
	if rows, err := (Naive{}).Eval(bg, mustParse(t, `node(?n), ?n != ?skip, not edge(?n, ?skip) => ?n`), b, Bind(map[Var]ns.Value{"skip": ns.S("b")})); err != nil || col(rows, "n") != "c,d,x" {
		t.Errorf("bound in a comparison and a negation: %v, %v; want c,d,x", rows, err)
	}
	if _, err := (SemiNaive{}).Eval(bg, mustParse(t, `edge(?a, ?b)`), b, Bind(map[Var]ns.Value{"z": ns.S("a"), "y": ns.S("b")})); err == nil || err.Error() != "query: cannot bind ?y, ?z: the goal does not use it" {
		t.Errorf("binding unused variables: err = %v", err)
	}
	rows, err = SemiNaive{}.Eval(bg, mustParse(t, `edge(?a, ?b) => ?a, count(?b)`), b, Bind(map[Var]ns.Value{"a": ns.S("b")}))
	if err != nil || len(rows) != 1 || rows[0].Bind["a"].S != "b" || rows[0].Bind["count(b)"].S != "1" {
		t.Errorf("a bound group key: %v, %v; want a=b, count 1", rows, err)
	}
}

// A variable the host binds still anchors a negation, as it does written in the goal: anchoring is
// checked on the goal before Bind makes the variable a constant, and on the rules before a rewrite
// inlines or renames them (#92). control: a negation anchored to nothing is still refused, naming the
// variable as written, by every evaluator and by ValidateBound.
func TestABoundVariableStillAnchorsANegation(t *testing.T) {
	v := std(graph())
	bind := Bind(map[Var]ns.Value{"n": ns.S("d")})
	for _, text := range []string{
		`node(?n), not edge(?n, ?y) => ?n`,
		`r(?x) :- node(?x), not edge(?x, ?y); r(?n) => ?n`,
	} {
		rows, err := both(mustParse(t, text), baseFor(v), bind)
		if err != nil || col(rows, "n") != "d" {
			t.Errorf("%s with ?n = d: %v, %v; want d (it has no edge out)", text, col(rows, "n"), err)
		}
		if err := ValidateBound(mustParse(t, text), v, "n"); err != nil {
			t.Errorf("%s: ValidateBound = %v, want nil", text, err)
		}
	}
	for _, text := range []string{
		`node(?n), not edge(?m, ?y) => ?n`,
		`r(?x) :- node(?x), not edge(?m, ?y); r(?n) => ?n`,
	} {
		_, err := both(mustParse(t, text), baseFor(v), bind)
		if err == nil || !strings.Contains(err.Error(), "(?m appears only inside") {
			t.Errorf("control: %s = %v, want the unanchored refusal naming ?m", text, err)
		}
		if verr := ValidateBound(mustParse(t, text), v, "n"); fmt.Sprint(verr) != fmt.Sprint(err) {
			t.Errorf("control: %s: ValidateBound = %v, want Eval's %v", text, verr, err)
		}
	}
}

// A variable the host binds is a constant to Validate as it is to Eval, so a goal that uses it only
// inside a `not`, or as the input a generator's mode needs, validates when it is bound and is refused
// when it isn't (#61).
func TestValidatingAGoalTheHostBinds(t *testing.T) {
	v := std(graph())
	b := baseFor(v)
	q := mustParse(t, `not node(?n) => ?n order by ?n`)
	const unanchored = `negated relation "node" shares no variable with the rest of the query`
	if err := Validate(q, v); err == nil || !strings.Contains(err.Error(), unanchored) {
		t.Errorf("unbound: Validate = %v, want the unanchored-negation refusal", err)
	}
	if err := ValidateBound(q, v, "n"); err != nil {
		t.Errorf("bound: ValidateBound = %v, want nil", err)
	}
	for _, ev := range evaluators {
		for _, c := range []struct {
			val  string
			want int
		}{{"a", 0}, {"zzz", 1}} {
			rows, err := ev.Eval(bg, q, b, Bind(map[Var]ns.Value{"n": ns.S(c.val)}))
			lit, lerr := ev.Eval(bg, mustParse(t, `not node("`+c.val+`")`), b)
			if err != nil || lerr != nil || len(rows) != c.want || len(lit) != c.want {
				t.Errorf("%T, ?n = %q: %d rows (%v), the constant written in %d (%v); want %d", ev, c.val, len(rows), err, len(lit), lerr, c.want)
			}
		}
	}

	_, eerr := SemiNaive{}.Eval(bg, q, b, Bind(map[Var]ns.Value{"z": ns.S("a"), "y": ns.S("b")}))
	if err := ValidateBound(q, v, "z", "y"); eerr == nil || fmt.Sprint(err) != fmt.Sprint(eerr) {
		t.Errorf("unused variables: ValidateBound = %v, Eval = %v; want Eval's refusal from both", err, eerr)
	}

	empty := ns.MustVocabulary(ns.NewMemSource())
	if err := ValidateBound(mustParse(t, `nope(?x, ?y) => ?x, ?y order by ?x`), empty, "x"); err != nil {
		t.Errorf("no relations installed, ordered by a bound column: %v, want nil", err)
	}

	gv := std(line(4))
	walker(t, gv, [][]bool{{true, false}})
	walk := mustParse(t, `walk(?s, ?e) => ?e`)
	if err := Validate(walk, gv); err == nil {
		t.Errorf("control: walk with nothing bound validated, want the mode refusal")
	}
	if err := ValidateBound(walk, gv, "s"); err != nil {
		t.Errorf("walk with ?s bound: ValidateBound = %v, want nil", err)
	}
	rows, err := SemiNaive{}.Eval(bg, walk, baseFor(gv), Bind(map[Var]ns.Value{"s": ns.S("v1")}))
	if err != nil || col(rows, "e") != "v2,v3" {
		t.Errorf("walk with ?s bound: %v, %v; want v2,v3", col(rows, "e"), err)
	}
}

// A variable the host binds in a closed-Domain argument validates: the placeholder ValidateBound
// binds is absent, and an absent value is no misspelling to refuse (#68).
func TestValidatingABoundClosedDomainArgument(t *testing.T) {
	src := ns.NewMemSource().DeclareSchema("role", ns.Schema{Arity: 2, Labels: []string{"node", "role"}, Types: []ns.ArgType{{}, {Domain: []string{"source", "sink"}}}})
	src.Add("role", ns.Tuple{Vals: []ns.Value{ns.S("a"), ns.S("sink")}})
	src.Add("role", ns.Tuple{Vals: []ns.Value{ns.S("b"), ns.Absent()}})
	v := std(src)
	q := mustParse(t, `role(?n, ?r) => ?n`)
	if err := Validate(q, v); err != nil {
		t.Errorf("control: unbound, Validate = %v, want nil, so a refusal below comes from binding ?r", err)
	}
	if err := ValidateBound(q, v, "r"); err != nil {
		t.Errorf("?r bound: ValidateBound = %v, want nil", err)
	}
	if rows, err := both(q, baseFor(v), Bind(map[Var]ns.Value{"r": ns.S("sink")})); err != nil || col(rows, "n") != "a" {
		t.Errorf("?r bound to sink: %v, %v; want a", col(rows, "n"), err)
	}
	if rows, err := both(q, baseFor(v), Bind(map[Var]ns.Value{"r": ns.Absent()})); err != nil || col(rows, "n") != "b" {
		t.Errorf("?r bound absent: %v, %v; want b, the one absent role", col(rows, "n"), err)
	}
	misspelled := mustParse(t, `role(?n, "sinkk") => ?n`)
	if err := ValidateBound(misspelled, v); err == nil || !strings.Contains(err.Error(), `role's "role" argument cannot be "sinkk"`) {
		t.Errorf("control: a misspelled constant: %v, want the Domain refusal", err)
	}
}
