package downloads

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// raw encodes a patch as a live message.
func raw(t *testing.T, p Patch) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Two patches in sequence must merge into one over both windows: newer fields win, a removed key leaves the upserts, a key back after removal is upserted again, and totals are the newest.
func TestMergePatch_Sequence(t *testing.T) {
	a := Patch{Kind: KindPatch, Boot: "b", Client: 3, FromRev: 4, ToRev: 5,
		Upsert: []map[string]any{{"key": "3:a", "progress": 0.1, "speed": 5.0}, {"key": "3:b", "progress": 0.2}},
		Remove: []string{"3:c"}, Totals: &PatchTotals{Speed: 1}}
	b := Patch{Kind: KindPatch, Boot: "b", Client: 3, FromRev: 5, ToRev: 7,
		Upsert: []map[string]any{{"key": "3:a", "progress": 0.4}, {"key": "3:c", "name": "back"}},
		Remove: []string{"3:b"}, Totals: &PatchTotals{Speed: 9}}
	var m Patch
	if err := json.Unmarshal(MergePatch(raw(t, a), raw(t, b)), &m); err != nil {
		t.Fatal(err)
	}
	if m.FromRev != 4 || m.ToRev != 7 || m.Totals.Speed != 9 {
		t.Fatalf("revs/totals %+v", m)
	}
	ups := map[string]map[string]any{}
	for _, u := range m.Upsert {
		ups[u["key"].(string)] = u
	}
	if len(ups) != 2 || ups["3:a"]["progress"] != 0.4 || ups["3:a"]["speed"] != 5.0 || ups["3:c"]["name"] != "back" || ups["3:b"] != nil {
		t.Fatalf("upserts %+v", m.Upsert)
	}
	if len(m.Remove) != 1 || m.Remove[0] != "3:b" {
		t.Fatalf("removes %+v", m.Remove)
	}
	// Out of sequence, another client or a health event: the newer one stays.
	c := b
	c.FromRev = 6
	if got := MergePatch(raw(t, a), raw(t, c)); string(got) != string(raw(t, c)) {
		t.Fatalf("out of sequence merged: %s", got)
	}
	h := Patch{Kind: KindHealth, Client: 3, Health: "slow"}
	if got := MergePatch(raw(t, a), raw(t, h)); string(got) != string(raw(t, h)) {
		t.Fatalf("health merged: %s", got)
	}
}

// Changes inside one window must become one patch from the first base to the newest rev, built from whole states so a field that went and came back is not sent.
func TestPublisher_BatchesChangesInOneWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	st := &Store{Now: func() time.Time { return now }}
	type built struct {
		from, to  uint64
		prev, cur map[string]Job
	}
	var got []built
	p := &SnapshotPublisher{Store: st, Now: func() time.Time { return now },
		Build: func(_ context.Context, _ int64, from, to uint64, prev, cur map[string]Job) {
			got = append(got, built{from, to, prev, cur})
		}}
	read := func(jobs ...Job) {
		_, prev, _ := st.Apply(1, 1, "", jobs, now)
		p.Note(1, st.Rev(1)-1, prev)
	}
	read(Job{ID: "a", Progress: 0.1})
	read(Job{ID: "a", Progress: 0.2})
	read(Job{ID: "a", Progress: 0.3}, Job{ID: "b"})
	p.Flush(context.Background(), false)
	if len(got) != 0 {
		t.Fatalf("flushed before the window: %+v", got)
	}
	now = now.Add(PublishWindow)
	p.Flush(context.Background(), false)
	if len(got) != 1 || got[0].from != 0 || got[0].to != 3 || len(got[0].cur) != 2 || got[0].prev != nil {
		t.Fatalf("batch %+v", got)
	}
	read(Job{ID: "a", Progress: 0.4})
	now = now.Add(PublishWindow)
	p.Flush(context.Background(), false)
	if len(got) != 2 || got[1].from != 3 || got[1].to != 4 || got[1].prev["a"].Progress != 0.3 {
		t.Fatalf("second batch %+v", got[1])
	}
	// With nothing new, nothing is sent.
	now = now.Add(PublishWindow)
	p.Flush(context.Background(), true)
	if len(got) != 2 {
		t.Fatalf("empty window sent %+v", got)
	}
}
