package dlsync

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
)

func jobSnapshot(js ...downloader.Job) map[string]downloader.Job {
	m := map[string]downloader.Job{}
	for _, j := range js {
		m[j.ID] = j
	}
	return m
}

func TestDiffAddChangeRemoveTransition(t *testing.T) {
	prev := jobSnapshot(
		downloader.Job{ID: "a", State: downloader.StateDownloading, Progress: 0.2, Speed: 100},
		downloader.Job{ID: "b", State: downloader.StateDownloading, Progress: 0.5},
		downloader.Job{ID: "c", State: downloader.StatePaused},
		downloader.Job{ID: "f", Files: []downloader.File{{Name: "x"}}},
	)
	cur := jobSnapshot(
		downloader.Job{ID: "a", State: downloader.StateDownloading, Progress: 0.3, Speed: 100},
		downloader.Job{ID: "b", State: downloader.StateFinished, Progress: 1},
		downloader.Job{ID: "d", State: downloader.StateQueued},
		downloader.Job{ID: "f", Files: []downloader.File{{Name: "y"}}},
	)
	c := Diff(prev, cur)
	want := Changes{
		Added:       []string{"d"},
		Removed:     []string{"c"},
		Changed:     []FieldChange{{ID: "a", Fields: []string{"Progress"}}, {ID: "b", Fields: []string{"Progress", "State"}}, {ID: "f", Fields: []string{"Files"}}},
		Transitions: []Transition{{ID: "b", From: downloader.StateDownloading, To: downloader.StateFinished}},
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

func TestDiffLimitsPointer(t *testing.T) {
	a := downloader.Job{ID: "a", Limits: &downloader.ShareLimits{Ratio: 1}}
	b := downloader.Job{ID: "a", Limits: &downloader.ShareLimits{Ratio: 1}}
	if !Diff(jobSnapshot(a), jobSnapshot(b)).Empty() {
		t.Fatal("equal limits behind different pointers counted as a change")
	}
	b.Limits = &downloader.ShareLimits{Ratio: 2}
	if c := Diff(jobSnapshot(a), jobSnapshot(b)); len(c.Changed) != 1 || c.Changed[0].Fields[0] != "Limits" {
		t.Fatalf("%+v", c)
	}
}

func TestStoreNilSafeAndFreshness(t *testing.T) {
	var nilStore *Store
	if s := nilStore.Current(1); s.Complete || s.Fresh || s.Items != nil || nilStore.Rev(1) != 0 {
		t.Fatalf("nil store %+v", s)
	}
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s := &Store{Floor: 2 * time.Second, Now: func() time.Time { return now }}
	if c := s.Current(1); c.Complete || c.Fresh {
		t.Fatal("unknown client is complete")
	}
	s.Update(1, 0, []downloader.Job{{ID: "a"}}, now)
	if c := s.Current(1); !c.Complete || !c.Fresh || c.Rev != 1 || len(c.Items) != 1 {
		t.Fatalf("%+v", c)
	}
	now = now.Add(6 * time.Second) // 3 x floor
	if !s.Current(1).Fresh {
		t.Fatal("stale at exactly 3 x floor")
	}
	now = now.Add(time.Millisecond)
	if c := s.Current(1); c.Fresh || !c.Complete {
		t.Fatalf("fresh past 3 x floor: %+v", c)
	}
	// A slow floor is capped by the 2 minute ceiling.
	slow := &Store{Floor: 5 * time.Minute, Now: func() time.Time { return now }}
	slow.Update(1, 0, nil, now.Add(-FreshCeiling-time.Second))
	if slow.Current(1).Fresh {
		t.Fatal("ceiling not applied")
	}
	// Copies: changing the returned map does not touch the store.
	c := s.Current(1)
	delete(c.Items, "a")
	if len(s.Current(1).Items) != 1 {
		t.Fatal("Current handed out the store's map")
	}
}

func TestStoreRevMovesOnlyOnChangeAndFailKeepsItems(t *testing.T) {
	at := time.Now()
	s := &Store{}
	s.Update(1, 0, []downloader.Job{{ID: "a", Progress: 0.1}}, at)
	s.Update(1, 0, []downloader.Job{{ID: "a", Progress: 0.1}}, at)
	if s.Rev(1) != 1 {
		t.Fatalf("rev %d after an unchanged read", s.Rev(1))
	}
	ch := s.Update(1, 0, []downloader.Job{{ID: "a", Progress: 0.2}}, at)
	if s.Rev(1) != 2 || len(ch.Changed) != 1 {
		t.Fatalf("rev %d %+v", s.Rev(1), ch)
	}
	s.Update(2, 0, []downloader.Job{{ID: "z"}}, at)
	if s.Rev(1) != 2 || s.Rev(2) != 1 {
		t.Fatal("revs are not per client")
	}
	s.Fail(1, 0)
	if c := s.Current(1); c.Complete || c.Fresh || len(c.Items) != 1 {
		t.Fatalf("after fail %+v", c)
	}
	// Generation change: the next whole read is diffed against the old
	// items, so a removal in the gap is still seen.
	s.Reset(1, 1)
	ch = s.Update(1, 1, nil, at)
	if c := s.Current(1); c.Gen != 1 || !c.Complete || len(ch.Removed) != 1 {
		t.Fatalf("after reset %+v %+v", c, ch)
	}
}

type fakeMain struct {
	answers []string
	rids    []int64
	err     error
}

func (f *fakeMain) SyncMain(_ context.Context, rid int64) (downloader.MainData, error) {
	f.rids = append(f.rids, rid)
	if f.err != nil {
		return downloader.MainData{}, f.err
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
		return downloader.MainData{}, err
	}
	return downloader.MainData{Rid: raw.Rid, FullUpdate: raw.Full, Torrents: raw.Torrents, Removed: raw.Removed, Bytes: len(a)}, nil
}

const (
	full1  = `{"rid":1,"full_update":true,"torrents":{"AAAA000000000000000000000000000000000000":{"name":"A","state":"downloading","progress":0.1},"bbbb000000000000000000000000000000000000":{"name":"B","state":"pausedDL"}}}`
	delta2 = `{"rid":2,"torrents":{"AAAA000000000000000000000000000000000000":{"progress":0.5}},"torrents_removed":["bbbb000000000000000000000000000000000000"]}`
)

func ids(js []downloader.Job) []string {
	var out []string
	for _, j := range js {
		out = append(out, j.ID)
	}
	sort.Strings(out)
	return out
}

func TestCursorsFullThenDeltaAndGenerationReset(t *testing.T) {
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

func TestCursorsSharedSessionAndParseErrorDropsCursor(t *testing.T) {
	var c Cursors
	f := &fakeMain{answers: []string{full1}} // every answer full although we send a rid
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
	f.err = downloader.ErrSyncParse
	if _, err := c.Poll(ctx, 1, 0, f); !errors.Is(err, downloader.ErrSyncParse) {
		t.Fatal(err)
	}
	if c.Rid(1, 0) != 0 {
		t.Fatal("cursor kept after a garbled answer")
	}
}

func openDB(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
