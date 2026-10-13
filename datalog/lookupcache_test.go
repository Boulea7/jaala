package datalog

import (
	"reflect"
	"sync"
	"testing"
)

// versionedLooking is a looking Source reporting a version a test moves.
type versionedLooking struct {
	*looking
	v string
}

func (s *versionedLooking) Version() string { return s.v }

// A second query on a Base asks the Source nothing it already asked, in every evaluator, and answers
// as the first did, batched or not (#212). control: with the cache off it asks again.
func TestASecondQueryAsksTheSourceNothingItAlreadyAsked(t *testing.T) {
	v := std(fanned(40))
	for _, text := range []string{fannedQueries[0].text, fannedQueries[1].text, `attr("b3", ?v) => ?v`} {
		q := mustParse(t, text)
		for _, ev := range evaluators {
			for _, limit := range []int{defaultLookupLimit, 0} {
				src := batchingAt(fanned(40))
				b := MustBase(v, src)
				b.LimitLookupCache(limit)
				first, err := ev.Eval(bg, q, b)
				if err != nil {
					t.Fatal(err)
				}
				asked, batches := src.looked("attr")+src.looked("edge"), len(src.batches)
				second, err := ev.Eval(bg, q, b)
				again := src.looked("attr") + src.looked("edge") - asked + len(src.batches) - batches
				if err != nil || !reflect.DeepEqual(binds(second), binds(first)) {
					t.Errorf("%T %s: second answer %v, first %v: %v", ev, text, binds(second), binds(first), err)
				}
				if limit > 0 && again != 0 {
					t.Errorf("%T %s: the second query made %d calls, want none", ev, text, again)
				}
				if limit == 0 && asked > 0 && again == 0 {
					t.Errorf("control: %T %s with the cache off: %d calls, then none", ev, text, asked)
				}
				if ev == (SemiNaive{}) && asked == 0 {
					t.Errorf("control: the planned SemiNaive made no calls for %s", text)
				}
			}
		}
	}
}

// The cache keeps at most its limit in tuples, dropping the least recently used result first, and keeps
// a result that matched nothing as one tuple, since it saves a call too.
func TestTheLookupCacheDropsTheLeastRecentlyUsedFirst(t *testing.T) {
	src := lookingAt(fanned(5))
	b := MustBase(std(fanned(5)), src)
	b.LimitLookupCache(2)
	ask := func(id string) int {
		t.Helper()
		before := src.looked("attr")
		if _, err := (SemiNaive{}).Eval(bg, mustParse(t, `attr("`+id+`", ?v) => ?v`), b); err != nil {
			t.Fatal(err)
		}
		return src.looked("attr") - before
	}
	for _, step := range []struct {
		id    string
		calls int
	}{{"b0", 1}, {"b1", 1}, {"b0", 0}, {"none", 1}, {"b0", 0}, {"b1", 1}, {"none", 1}, {"none", 0}} {
		if got := ask(step.id); got != step.calls {
			t.Errorf("asking %s made %d calls, want %d", step.id, got, step.calls)
		}
	}
	b.LimitLookupCache(0)
	if got := ask("none"); got != 1 {
		t.Errorf("after lowering the limit to 0, asking none again made %d calls, want 1", got)
	}
}

// A Versioned Source whose version moves, and Forget, drop what the Base kept, and an Eval that began
// before a drop can't store into the cache after it.
func TestTheLookupCacheIsDroppedWithTheBasesOtherCaches(t *testing.T) {
	q := mustParse(t, `attr("b1", ?v) => ?v`)
	src := &versionedLooking{looking: lookingAt(fanned(3)), v: "1"}
	b := MustBase(std(fanned(3)), src)
	calls := func() int {
		t.Helper()
		before := src.looked("attr")
		if _, err := (SemiNaive{}).Eval(bg, q, b); err != nil {
			t.Fatal(err)
		}
		return src.looked("attr") - before
	}
	if got := []int{calls(), calls()}; !reflect.DeepEqual(got, []int{1, 0}) {
		t.Errorf("control: calls %v, want [1 0]", got)
	}
	src.v = "2"
	if got := calls(); got != 1 {
		t.Errorf("after the version moved: %d calls, want 1", got)
	}
	b.Forget()
	if got := calls(); got != 1 {
		t.Errorf("after Forget: %d calls, want 1", got)
	}
	gen := b.lookups.generation()
	b.Forget()
	b.lookups.put(gen, "k", nil)
	if _, ok := b.lookups.get("k"); ok {
		t.Error("an Eval begun before Forget stored into the cache after it")
	}
}

// Explain counts a call answered from an earlier query's lookup as kept, apart from this Eval's own
// hits, and an Eval answered that way reports what a fresh Base would have cost.
func TestExplainCountsKeptLookups(t *testing.T) {
	q := mustParse(t, fannedQueries[0].text)
	b := MustBase(std(fanned(40)), batchingAt(fanned(40)))
	var first, second Report
	if _, err := (SemiNaive{}).Eval(bg, q, b, Explain(&first)); err != nil {
		t.Fatal(err)
	}
	if _, err := (SemiNaive{}).Eval(bg, q, b, Explain(&second)); err != nil {
		t.Fatal(err)
	}
	kept := int64(0)
	for _, s := range second.Sources {
		kept += s.Kept
	}
	if kept != 41 || second.Lookups != 0 || second.Batches != 0 {
		t.Errorf("second Eval: %d kept, %d lookups, %d batches; want 41, 0 and 0\n%s", kept, second.Lookups, second.Batches, &second)
	}
	if c := second.Cold; c == nil || c.Lookups != first.Lookups || c.Batches != first.Batches || c.Fetched != first.Fetched {
		t.Errorf("second Eval's cold cost %+v, want the first's lookups %d, batches %d, fetched %d", c, first.Lookups, first.Batches, first.Fetched)
	}
	if first.Cold == nil || first.Cold.Lookups != first.Lookups {
		t.Errorf("control: first Eval's cold %+v", first.Cold)
	}
}

// Evals sharing one Base's lookup cache run concurrently; -race checks it.
func TestConcurrentEvalsShareTheLookupCache(t *testing.T) {
	b := MustBase(std(fanned(40)), batchingAt(fanned(40)))
	b.LimitLookupCache(10)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := mustParse(t, fannedQueries[i%2].text)
			if _, err := (SemiNaive{}).Eval(bg, q, b); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// The keys a negation looks up come in one batch over the positive body's bindings (#230).
// control: a Source that doesn't batch is asked once per binding.
func TestANegatedLookupIsBatched(t *testing.T) {
	q := mustParse(t, `edge("a", ?x), not attr(?x, "v0") => ?x`)
	v := std(fanned(40))
	want, werr := both(q, baseFor(v))
	src := batchingAt(fanned(40))
	got, err := both(q, MustBase(v, src))
	if err != nil || werr != nil || !reflect.DeepEqual(binds(got), binds(want)) || len(want) == 0 {
		t.Fatalf("batched %v %v, whole %v %v", binds(got), err, binds(want), werr)
	}
	src = batchingAt(fanned(40))
	if _, err := (SemiNaive{}).Eval(bg, q, MustBase(v, src)); err != nil || src.looked("attr") != 0 || !reflect.DeepEqual(src.batches, []string{"attr×40"}) {
		t.Errorf("negated attr: %d lookups, batches %v, %v", src.looked("attr"), src.batches, err)
	}
	plain := lookingAt(fanned(40))
	if _, err := (SemiNaive{}).Eval(bg, q, MustBase(v, plain)); err != nil || plain.looked("attr") != 40 {
		t.Errorf("control: unbatched, negated attr looked up %d times, want 40: %v", plain.looked("attr"), err)
	}
	var r, p Report
	if _, err := (SemiNaive{}).Eval(bg, q, MustBase(v, batchingAt(fanned(40))), Explain(&r)); err != nil {
		t.Fatal(err)
	}
	if _, err := (SemiNaive{}).Eval(bg, q, MustBase(v, lookingAt(fanned(40))), Explain(&p)); err != nil {
		t.Fatal(err)
	}
	for i, l := range r.Goal.Literals {
		if pl := p.Goal.Literals[i]; l.Reached != pl.Reached || l.Passed != pl.Passed {
			t.Errorf("%s reached %d and passed %d, %d and %d unbatched", l.Literal, l.Reached, l.Passed, pl.Reached, pl.Passed)
		}
	}
	if !reflect.DeepEqual(r.Goal.Literals[1].Access, []string{"not batched lookup (id, value)", "not lookup (id, value)"}) {
		t.Errorf("the negation's access %v", r.Goal.Literals[1].Access)
	}
}
