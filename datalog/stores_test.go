package datalog

import (
	"context"
	"fmt"
	"math/bits"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/panyam/jaala/ns"
)

// The store-federation mission's exercise (#218, #219): a federation of stores shaped like a host's
// (Declaire's one store per repo fragment), each answering lookups from the patterns it indexes and
// scanning for the rest, asked questions shaped like a host's. BenchmarkStores reports what each
// question costs the stores as their number grows; TestStoresExercise is its smoke test in CI.

// storeNodes is each repo's node count. A node n<k> calls n<2k+1> and n<2k+2> in its own repo, a
// tree, and every twentieth calls a leaf of the next repo, so a walk from one node reaches its repo
// and a few leaves, however many repos there are.
const storeNodes = 200

// storeChain is how many nodes each repo's next chain (n0, n1, ...) links, a walk as deep as it is
// long, so a recursive question's cost per level shows (#231).
const storeChain = 60

// storeLatencyMs is the round trip a store call stands for, SQLite over OPFS or a network, which
// the benchmark reports from the calls rather than sleeping.
const storeLatencyMs = 2

// A store is one repo's facts behind a LookupSource with an index catalog: a lookup binding every
// position of an indexed pattern reads that pattern's bucket, and any other scans the relation. It
// counts the calls made to it, the calls that scanned, and the tuples it scanned.
type store struct {
	*ns.MemSource
	indexed map[string][][]int
	buckets map[string][]ns.Tuple
	// uncatalogued hides the catalog from the planner: Indexed says every pattern is indexed, as a
	// LookupSource that doesn't implement ns.IndexedLookups is taken to.
	uncatalogued bool

	mu                        sync.Mutex
	calls, unindexed, scanned int
}

// storeCatalog is what each store indexes: node, attr and edge by their first argument (an id, and
// an edge's source). Edges by their target are deliberately missing, as a real store's might be.
var storeCatalog = map[string][][]int{"node": {{0}}, "attr": {{0}}, "edge": {{0}}, "next": {{0}}}

func repoStore(repo, repos int) *store {
	m := ns.NewMemSource().
		Declare("node", "id", "kind").
		Declare("attr", "id", "key", "value").
		Declare("edge", "src", "kind", "dst").
		Declare("next", "src", "dst")
	id := func(r, k int) ns.Value { return ns.S(fmt.Sprintf("repo%d/n%d", r, k)) }
	for k := range storeNodes {
		m.Add("node", ns.Tuple{Vals: []ns.Value{id(repo, k), ns.S("func")}})
		m.Add("attr", ns.Tuple{Vals: []ns.Value{id(repo, k), ns.S("name"), ns.S("n" + strconv.Itoa(k))}})
		m.Add("attr", ns.Tuple{Vals: []ns.Value{id(repo, k), ns.S("lines"), ns.S(strconv.Itoa(k % 50))}})
		for _, c := range []int{2*k + 1, 2*k + 2} {
			if c < storeNodes {
				m.Add("edge", ns.Tuple{Vals: []ns.Value{id(repo, k), ns.S("calls"), id(repo, c)}})
			}
		}
		if k+1 < storeChain {
			m.Add("next", ns.Tuple{Vals: []ns.Value{id(repo, k), id(repo, k+1)}})
		}
		if k%20 == 0 {
			m.Add("edge", ns.Tuple{Vals: []ns.Value{id(repo, k), ns.S("calls"), id((repo+1)%repos, storeNodes-1-k/20)}})
		}
	}
	s := &store{MemSource: m, indexed: storeCatalog, buckets: map[string][]ns.Tuple{}}
	for rel, patterns := range storeCatalog {
		for _, p := range patterns {
			for _, t := range m.Tuples(rel) {
				k := bucketKey(rel, p, func(i int) ns.Value { return t.Vals[i] })
				s.buckets[k] = append(s.buckets[k], t)
			}
		}
	}
	return s
}

func bucketKey(rel string, pattern []int, val func(int) ns.Value) string {
	parts := []string{rel, fmt.Sprint(pattern)}
	for _, i := range pattern {
		parts = append(parts, val(i).S)
	}
	return strings.Join(parts, "\x00")
}

// indexes reports the catalog pattern a lookup binding bound can read, if any.
func (s *store) indexes(rel string, bound func(int) bool) ([]int, bool) {
	for _, p := range s.indexed[rel] {
		if !slices.ContainsFunc(p, func(i int) bool { return !bound(i) }) {
			return p, true
		}
	}
	return nil, false
}

// Indexed implements ns.IndexedLookups from the catalog.
func (s *store) Indexed(rel string, bound []int) bool {
	_, ok := s.indexes(rel, func(i int) bool { return slices.Contains(bound, i) })
	return ok || s.uncatalogued
}

// Tuples counts a whole read as a call that scanned.
func (s *store) Tuples(rel string) []ns.Tuple {
	ts := s.MemSource.Tuples(rel)
	s.mu.Lock()
	s.calls++
	s.unindexed++
	s.scanned += len(ts)
	s.mu.Unlock()
	return ts
}

func (s *store) Lookup(_ context.Context, rel string, bound map[int]ns.Value) ([]ns.Tuple, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	out, scan := s.match(rel, bound)
	if scan {
		s.scanned += len(s.MemSource.Tuples(rel))
		s.unindexed++
	}
	return out, nil
}

// match is rel's facts matching bound, from an index when the catalog has one, and whether it scanned.
func (s *store) match(rel string, bound map[int]ns.Value) ([]ns.Tuple, bool) {
	if p, ok := s.indexes(rel, func(i int) bool { _, ok := bound[i]; return ok }); ok {
		return s.buckets[bucketKey(rel, p, func(i int) ns.Value { return bound[i] })], false
	}
	var out []ns.Tuple
	for _, t := range s.MemSource.Tuples(rel) {
		match := true
		for i, v := range bound {
			match = match && t.Vals[i].S == v.S
		}
		if match {
			out = append(out, t)
		}
	}
	return out, true
}

// A batchingStore is a store answering a batch of lookups in one call (ns.BatchLookupSource, #153),
// as a database asked `WHERE id IN (...)` does; an unindexed batch scans the relation once.
type batchingStore struct{ *store }

func (s batchingStore) LookupBatch(_ context.Context, rel string, positions []int, values [][]ns.Value) ([][]ns.Tuple, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	out := make([][]ns.Tuple, len(values))
	scanned := false
	for k, vals := range values {
		bound := map[int]ns.Value{}
		for j, p := range positions {
			bound[p] = vals[j]
		}
		var scan bool
		out[k], scan = s.match(rel, bound)
		scanned = scanned || scan
	}
	if scanned {
		s.scanned += len(s.MemSource.Tuples(rel))
		s.unindexed++
	}
	return out, nil
}

func (s *store) reset() {
	s.mu.Lock()
	s.calls, s.unindexed, s.scanned = 0, 0, 0
	s.mu.Unlock()
}

// repoStores is n repos' stores, each held by the repo's id prefix wherever an argument is a
// repo's own id, and answering a batch of lookups in one call when batching. An edge lives in its
// source's repo, so its target names no store.
func repoStores(n int, batching bool) ([]*store, *ns.Federation) {
	stores := make([]*store, n)
	members := make([]ns.Member, n)
	for i := range n {
		stores[i] = repoStore(i, n)
		var src ns.Source = stores[i]
		if batching {
			src = batchingStore{stores[i]}
		}
		members[i] = ns.Member{
			Name:   fmt.Sprintf("repo%d", i),
			Source: src,
			Holds:  ns.HoldsPrefix(fmt.Sprintf("repo%d/", i), map[string][]int{"node": {0}, "attr": {0}, "edge": {0}, "next": {0}}),
		}
	}
	f, err := ns.NewFederation(members...)
	if err != nil {
		panic(err)
	}
	return stores, f
}

// storeQuestions are the exercise's questions, about repo0, which every N has. callees-with-attr
// binds attr's key and value, more than edge's source, but no store indexes attr by them, so a
// planner that knows the catalog reads edge first (#154).
var storeQuestions = []struct{ name, text string }{
	{"attrs-by-id", `attr("repo0/n3", ?k, ?v) => ?k, ?v`},
	{"two-hops", `edge("repo0/n20", _, ?m), edge(?m, _, ?o) => ?o`},
	{"callers", `edge(?s, _, "repo0/n3") => ?s`},
	{"callees-with-attr", `attr(?m, "lines", "7"), edge("repo0/n3", _, ?m) => ?m`},
	{"reach", `reach(?a, ?b) :- edge(?a, _, ?b); reach(?a, ?c) :- reach(?a, ?b), edge(?b, _, ?c); reach("repo0/n0", ?x) => count(?x)`},
	{"deep-walk", `walk(?a, ?b) :- next(?a, ?b); walk(?a, ?c) :- walk(?a, ?b), next(?b, ?c); walk("repo0/n0", ?x) => count(?x)`},
}

// storeCost is what one ask of a question cost the stores.
type storeCost struct {
	asked, calls, unindexed, scanned int
}

func costOf(stores []*store) storeCost {
	var c storeCost
	for _, s := range stores {
		s.mu.Lock()
		if s.calls > 0 {
			c.asked++
		}
		c.calls += s.calls
		c.unindexed += s.unindexed
		c.scanned += s.scanned
		s.mu.Unlock()
		s.reset()
	}
	return c
}

// An uncatalogued storeSet's stores hide their catalog from the planner, as stores that don't say
// what they index.
func (s storeSet) uncatalogued() storeSet {
	for _, st := range s.stores {
		st.uncatalogued = true
	}
	return s
}

// A storeSet is n repos' stores, their federation and its vocabulary, built once so that what a
// question costs is measured apart from the fixture.
type storeSet struct {
	stores []*store
	fed    *ns.Federation
	v      *ns.Vocabulary
}

func newStoreSet(n int) storeSet {
	stores, f := repoStores(n, true)
	return storeSet{stores: stores, fed: f, v: std(f)}
}

// unbatchedStoreSet is newStoreSet over stores asked one lookup at a time, as before #153.
func unbatchedStoreSet(n int) storeSet {
	stores, f := repoStores(n, false)
	return storeSet{stores: stores, fed: f, v: std(f)}
}

// askTwice asks q on a fresh Base over the stores, set up by each of setup, then again on the same
// Base, and returns each ask's cost and the first ask's answer.
func (s storeSet) askTwice(q Query, setup ...func(*Base)) (first, second storeCost, rows []Row, err error) {
	costOf(s.stores)
	b := MustBase(s.v, s.fed)
	for _, f := range setup {
		f(b)
	}
	if rows, err = (SemiNaive{}).Eval(bg, q, b); err != nil {
		return
	}
	first = costOf(s.stores)
	if _, err = (SemiNaive{}).Eval(bg, q, b); err != nil {
		return
	}
	return first, costOf(s.stores), rows, nil
}

// BenchmarkStores is mission #219's exercise: each question over 1, 10 and 50 stores, reporting the
// stores it asked, its calls to them (round trips, and the latency they stand for), the calls an index
// couldn't answer and the tuples they scanned, what asking it again on the same Base called, and the
// peak heap in use.
func BenchmarkStores(b *testing.B) {
	for _, question := range storeQuestions {
		q := MustParse(question.text)
		for _, n := range []int{1, 10, 50} {
			b.Run(fmt.Sprintf("%s/stores=%d", question.name, n), func(b *testing.B) {
				set := newStoreSet(n)
				var first, second storeCost
				var peak uint64
				for i := 0; i < b.N; i++ {
					var err error
					peak = max(peak, peakHeapDuring(func() { first, second, _, err = set.askTwice(q) }))
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(first.asked), "stores-asked")
				b.ReportMetric(float64(first.calls), "store-calls")
				b.ReportMetric(float64(first.calls*storeLatencyMs), "latency-ms")
				b.ReportMetric(float64(first.unindexed), "unindexed-calls")
				b.ReportMetric(float64(first.scanned), "scanned")
				b.ReportMetric(float64(second.calls), "calls-again")
				b.ReportMetric(float64(peak)/1e6, "peak-MB")
			})
		}
	}
}

// The exercise's fixture can tell its cases apart: over three stores each question answers as one
// Source of the same facts does, and the costs the mission sets out to cut show (the checks marked
// control). The rest are the mission's targets met so far: an indexed pattern first (#154), a batch
// per call (#153), and nothing fetched for a repeated question (#212).
func TestStoresExercise(t *testing.T) {
	const n = 3
	set := newStoreSet(n)
	f := set.fed
	whole := ns.NewMemSource()
	for _, rel := range f.Relations() {
		s, _ := f.Schema(rel)
		whole.DeclareSchema(rel, s)
		for _, tu := range f.Tuples(rel) {
			whole.Add(rel, tu)
		}
	}
	costs := map[string]storeCost{}
	for _, question := range storeQuestions {
		q := mustParse(t, question.text)
		first, second, got, err := set.askTwice(q)
		if err != nil {
			t.Fatalf("%s: %v", question.name, err)
		}
		want, err := both(q, MustBase(std(whole), whole))
		if err != nil || !reflect.DeepEqual(binds(got), binds(want)) {
			t.Errorf("%s over %d stores: %v, over one source: %v %v", question.name, n, binds(got), binds(want), err)
		}
		if len(want) == 0 {
			t.Errorf("control: %s answers nothing", question.name)
		}
		if second.calls != 0 {
			t.Errorf("asking %s again made %d store calls, want none: the Base keeps what it looked up (#212)", question.name, second.calls)
		}
		uncached := func(b *Base) { b.LimitLookupCache(0) }
		if _, again, _, err := set.askTwice(q, uncached); err != nil || again.calls == 0 {
			t.Errorf("control: with the lookup cache off, asking %s again called no store: %v", question.name, err)
		}
		costs[question.name] = first
	}
	if c := costs["attrs-by-id"]; c.asked != 1 || c.unindexed != 0 {
		t.Errorf("control: a lookup by id asked %d stores with %d unindexed calls, want 1 and 0", c.asked, c.unindexed)
	}
	if c := costs["callers"]; c.asked != n || c.unindexed != n {
		t.Errorf("control: callers asked %d stores with %d unindexed calls, want %d and %d (#154)", c.asked, c.unindexed, n, n)
	}
	reach := mustParse(t, storeQuestions[4].text)
	if c, _, _, err := unbatchedStoreSet(n).askTwice(reach); err != nil || c.calls <= storeNodes {
		t.Errorf("control: over stores that don't batch, reach made %d store calls, want more than its repo's %d nodes (a call per demanded value): %v", c.calls, storeNodes, err)
	}
	if c := costs["deep-walk"]; c.calls < storeChain-1 {
		t.Errorf("control: a walk %d deep made %d calls, want one per level: batching can't go below that without the store walking itself (#231)", storeChain, c.calls)
	}
	if c, depth := costs["reach"], bits.Len(storeNodes); c.calls > c.asked*depth {
		t.Errorf("reach made %d calls to %d stores, want at most a batch per store per level of its %d-level tree (#153)", c.calls, c.asked, depth)
	}
	blind, _, _, err := newStoreSet(n).uncatalogued().askTwice(mustParse(t, storeQuestions[3].text))
	if err != nil || blind.unindexed != n {
		t.Errorf("control: without the catalog, callees with an attribute made %d unindexed calls, want %d (one scan per store): %v", blind.unindexed, n, err)
	}
	if c := costs["callees-with-attr"]; c.asked != 1 || c.unindexed != 0 {
		t.Errorf("callees with an attribute asked %d stores with %d unindexed calls, want 1 and 0: the planner should read edge by its source first (#154)", c.asked, c.unindexed)
	}
	if c := costs["two-hops"]; c.asked != 2 {
		t.Errorf("control: two hops from a node calling across repos asked %d stores, want 2", c.asked)
	}
}
