package datalog

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/panyam/jaala/ns"
)

// slowSource reads each relation slowly and counts the reads, so concurrent first reads overlap.
type slowSource struct {
	*ns.MemSource
	mu    sync.Mutex
	reads map[string]int
}

func (s *slowSource) Tuples(rel string) []ns.Tuple {
	s.mu.Lock()
	s.reads[rel]++
	s.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	return s.MemSource.Tuples(rel)
}

// lockingWalk is a walk over edges that serializes itself on one lock, as a host whose own engine has
// a single connection does (Declaire's SQLite graph.reach). It reads its own copy of the edges, so
// the Source's read count is the engine's alone.
func lockingWalk(t *testing.T, v *ns.Vocabulary, edges []ns.Tuple) {
	t.Helper()
	var mu sync.Mutex
	err := v.AddPredicate("walk", ns.Builtin{Arity: 2, Modes: [][]bool{{true, false}, {false, true}}, Gen: func(src ns.Source, args []ns.Arg, emit func([]ns.Value, []string) error) error {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range edges {
			if (args[0].Bound && e.Vals[0].S == args[0].Value.S) || (args[1].Bound && e.Vals[1].S == args[1].Value.S) {
				if err := emit(e.Vals, e.Cites); err != nil {
					return err
				}
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// Every kind of program a host runs at once on one Base, through every evaluator at once: lazy
// relation reads and index builds racing each other, a recursive derived relation, members of one
// shared module, a generator that takes a lock, and negation with aggregation. Each answer must equal
// the same program run alone, and each relation must be read from the Source once.
func TestConcurrentEvalsOnOneBase(t *testing.T) {
	src := &slowSource{MemSource: line(IndexMinTuples * 3), reads: map[string]int{}}
	v := std(src.MemSource)
	lockingWalk(t, v, src.MemSource.Tuples("edge"))
	if err := v.AddModule("go", LanguageName, `test(?f) :- node(?f); covers(?t, ?f) :- test(?t), walk(?t, ?f);`, ""); err != nil {
		t.Fatal(err)
	}
	programs := []string{
		`edge("v3", ?x) => ?x`,
		`node(?n), edge(?n, "v40") => ?n`,
		leftReach + `reach("v0", ?x) => count(?x)`,
		rightReach + `reach(?x, "v20") => ?x`,
		`go.covers(?t, "v10") => ?t`,
		`go.test(?f) => count(?f)`,
		`node(?n), not edge(?n, _) => ?n`,
	}
	evaluators := []Evaluator{Naive{}, SemiNaive{}, SemiNaive{WrittenOrder: true}}
	type job struct {
		q  Query
		ev Evaluator
	}
	var jobs []job
	want := map[int][]Row{}
	for _, text := range programs {
		for _, ev := range evaluators {
			j := job{mustParse(t, text), ev}
			rows, err := ev.Eval(j.q, MustBase(v, src.MemSource))
			if err != nil {
				t.Fatalf("serial %T %s: %v", ev, text, err)
			}
			want[len(jobs)] = rows
			jobs = append(jobs, j)
		}
	}
	shared := MustBase(v, src)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 4*len(jobs))
	for g := 0; g < 4; g++ {
		for i := range jobs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				got, err := jobs[i].ev.Eval(jobs[i].q, shared)
				if err != nil || !reflect.DeepEqual(got, want[i]) {
					errs <- fmt.Errorf("%T %v: got %v, %v; want %v", jobs[i].ev, jobs[i].q, got, err, want[i])
				}
			}((i + g*3) % len(jobs))
		}
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for rel, n := range src.reads {
		if n != 1 {
			t.Errorf("%s read from the Source %d times, want once", rel, n)
		}
	}
	if len(src.reads) == 0 {
		t.Error("control: the Source was never read, so this test proves nothing about reads")
	}
}
