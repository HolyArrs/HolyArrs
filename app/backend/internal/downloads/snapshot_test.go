package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"
)

// jobSnapshot indexes jobs by id, the shape Diff takes.
func jobSnapshot(js ...Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range js {
		m[j.ID] = j
	}
	return m
}

// Diff must report added, removed and changed items, and a state change as a transition.
func TestDiff_AddChangeRemoveTransition(t *testing.T) {
	prev := jobSnapshot(
		Job{ID: "a", State: StateDownloading, Progress: 0.2, Speed: 100},
		Job{ID: "b", State: StateDownloading, Progress: 0.5},
		Job{ID: "c", State: StatePaused},
		Job{ID: "f", Files: []File{{Name: "x"}}},
	)
	cur := jobSnapshot(
		Job{ID: "a", State: StateDownloading, Progress: 0.3, Speed: 100},
		Job{ID: "b", State: StateFinished, Progress: 1},
		Job{ID: "d", State: StateQueued},
		Job{ID: "f", Files: []File{{Name: "y"}}},
	)
	c := Diff(prev, cur)
	want := Changes{
		Added:       []string{"d"},
		Removed:     []string{"c"},
		Changed:     []FieldChange{{ID: "a", Fields: []string{"Progress"}}, {ID: "b", Fields: []string{"Progress", "State"}}, {ID: "f", Fields: []string{"Files"}}},
		Transitions: []Transition{{ID: "b", From: StateDownloading, To: StateFinished}},
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
	if !Diff(cur, cur).Empty() || !Diff(nil, nil).Empty() {
		t.Fatal("same input is not empty")
	}
	if got := Diff(nil, cur); len(got.Added) != 4 {
		t.Fatalf("first read %+v", got)
	}
}

// Diff must compare pointer fields by value, so equal limits behind two pointers are no change.
func TestDiff_PointerFieldsCompareByValue(t *testing.T) {
	a := Job{ID: "a", Limits: &ShareLimits{Ratio: 1}}
	b := Job{ID: "a", Limits: &ShareLimits{Ratio: 1}}
	if !Diff(jobSnapshot(a), jobSnapshot(b)).Empty() {
		t.Fatal("equal limits behind different pointers counted as a change")
	}
	b.Limits = &ShareLimits{Ratio: 2}
	if c := Diff(jobSnapshot(a), jobSnapshot(b)); len(c.Changed) != 1 || c.Changed[0].Fields[0] != "Limits" {
		t.Fatalf("%+v", c)
	}
}

// A nil Store must answer safely, and a read is fresh for three cadences up to FreshCeiling.
func TestStore_NilSafeAndFreshness(t *testing.T) {
	var nilStore *Store
	if s := nilStore.Current(1); s.Complete || s.Fresh || s.Items != nil || nilStore.Rev(1) != 0 {
		t.Fatalf("nil store %+v", s)
	}
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s := &Store{Floor: 2 * time.Second, Now: func() time.Time { return now }}
	if c := s.Current(1); c.Complete || c.Fresh {
		t.Fatal("unknown client is complete")
	}
	s.Update(1, 0, []Job{{ID: "a"}}, now)
	if c := s.Current(1); !c.Complete || !c.Fresh || c.Rev != 1 || len(c.Items) != 1 {
		t.Fatalf("%+v", c)
	}
	now = now.Add(6 * time.Second) // exactly 3 x floor
	if !s.Current(1).Fresh {
		t.Fatal("stale at exactly 3 x floor")
	}
	now = now.Add(time.Millisecond)
	if c := s.Current(1); c.Fresh || !c.Complete {
		t.Fatalf("fresh past 3 x floor: %+v", c)
	}
	// A slow floor is still capped by FreshCeiling.
	slow := &Store{Floor: 5 * time.Minute, Now: func() time.Time { return now }}
	slow.Update(1, 0, nil, now.Add(-FreshCeiling-time.Second))
	if slow.Current(1).Fresh {
		t.Fatal("ceiling not applied")
	}
	// Current returns a copy, so editing it must not change the store.
	c := s.Current(1)
	delete(c.Items, "a")
	if len(s.Current(1).Items) != 1 {
		t.Fatal("Current handed out the store's map")
	}
}

// The rev must move only when something changed, and a failed read must keep the items.
func TestStore_RevMovesOnlyOnChangeAndFailKeepsItems(t *testing.T) {
	at := time.Now()
	s := &Store{}
	s.Update(1, 0, []Job{{ID: "a", Progress: 0.1}}, at)
	s.Update(1, 0, []Job{{ID: "a", Progress: 0.1}}, at)
	if s.Rev(1) != 1 {
		t.Fatalf("rev %d after an unchanged read", s.Rev(1))
	}
	ch := s.Update(1, 0, []Job{{ID: "a", Progress: 0.2}}, at)
	if s.Rev(1) != 2 || len(ch.Changed) != 1 {
		t.Fatalf("rev %d %+v", s.Rev(1), ch)
	}
	s.Update(2, 0, []Job{{ID: "z"}}, at)
	if s.Rev(1) != 2 || s.Rev(2) != 1 {
		t.Fatal("revs are not per client")
	}
	s.Fail(1, 0)
	if c := s.Current(1); c.Complete || c.Fresh || len(c.Items) != 1 {
		t.Fatalf("after fail %+v", c)
	}
	// After a generation change the next whole read is diffed against the old
	// items, so a removal during the gap is still seen.
	s.Reset(1, 1)
	ch = s.Update(1, 1, nil, at)
	if c := s.Current(1); c.Gen != 1 || !c.Complete || len(ch.Removed) != 1 {
		t.Fatalf("after reset %+v %+v", c, ch)
	}
}

// fakeMain answers change-feed polls from a fixed list of bodies.
type fakeMain struct {
	answers []string
	rids    []int64
	err     error
}

func (f *fakeMain) SyncMain(_ context.Context, rid int64) (MainData, error) {
	f.rids = append(f.rids, rid)
	if f.err != nil {
		return MainData{}, f.err
	}
	a := f.answers[0]
	if len(f.answers) > 1 {
		f.answers = f.answers[1:]
	}
	var raw struct {
		Rid      int64                                 `json:"rid"`
		Full     bool                                  `json:"full_update"`
		Torrents map[string]map[string]json.RawMessage `json:"torrents"`
		Removed  []string                              `json:"torrents_removed"`
	}
	if err := json.Unmarshal([]byte(a), &raw); err != nil {
		return MainData{}, err
	}
	return MainData{Rid: raw.Rid, FullUpdate: raw.Full, Torrents: raw.Torrents, Removed: raw.Removed, Bytes: len(a)}, nil
}

const (
	full1  = `{"rid":1,"full_update":true,"torrents":{"AAAA000000000000000000000000000000000000":{"name":"A","state":"downloading","progress":0.1},"bbbb000000000000000000000000000000000000":{"name":"B","state":"pausedDL"}}}`
	delta2 = `{"rid":2,"torrents":{"AAAA000000000000000000000000000000000000":{"progress":0.5}},"torrents_removed":["bbbb000000000000000000000000000000000000"]}`
)

// ids returns the ids of jobs in order.
func ids(js []Job) []string {
	var out []string
	for _, j := range js {
		out = append(out, j.ID)
	}
	sort.Strings(out)
	return out
}

// The change feed must start full, then send deltas, and a new generation must start again from rid 0.
func TestCursors_FullThenDeltaAndGenerationReset(t *testing.T) {
	var c Cursors
	f := &fakeMain{answers: []string{full1, delta2}}
	ctx := context.Background()
	p, err := c.Poll(ctx, 1, 0, f)
	if err != nil || !p.Full || len(p.Jobs) != 2 {
		t.Fatalf("%v %+v", err, p)
	}
	p, err = c.Poll(ctx, 1, 0, f)
	if err != nil || p.Full || !reflect.DeepEqual(ids(p.Jobs), []string{"aaaa000000000000000000000000000000000000"}) || p.Jobs[0].Progress != 0.5 || p.Jobs[0].Name != "A" {
		t.Fatalf("%v %+v", err, p)
	}
	if !reflect.DeepEqual(f.rids, []int64{0, 1}) || c.Rid(1, 0) != 2 {
		t.Fatalf("rids %v", f.rids)
	}
	// A new engine generation starts from rid 0 and is not a shared session.
	f.answers = []string{full1}
	p, err = c.Poll(ctx, 1, 1, f)
	if err != nil || f.rids[2] != 0 || p.SharedSession {
		t.Fatalf("%v rids %v %+v", err, f.rids, p)
	}
	if c.Rid(1, 0) != 0 {
		t.Fatal("old generation's cursor kept")
	}
}

// Repeated full answers must flag a shared session, and a garbled answer must drop the cursor.
func TestCursors_SharedSessionAndParseErrorDropsCursor(t *testing.T) {
	var c Cursors
	f := &fakeMain{answers: []string{full1}} // every answer is full even though a rid is sent
	ctx := context.Background()
	var p MainPoll
	for range 5 {
		var err error
		if p, err = c.Poll(ctx, 1, 0, f); err != nil {
			t.Fatal(err)
		}
	}
	if !p.SharedSession {
		t.Fatal("4 full answers to a valid rid did not flag a shared session")
	}
	f.err = ErrSyncParse
	if _, err := c.Poll(ctx, 1, 0, f); !errors.Is(err, ErrSyncParse) {
		t.Fatal(err)
	}
	if c.Rid(1, 0) != 0 {
		t.Fatal("cursor kept after a garbled answer")
	}
}
