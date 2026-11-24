package downloads

import (
	"context"
	"testing"
)

// A download removed in the Downloader is settled at once when a person picks
// another Release, instead of after the usual misses; one still listed is not.
func TestSettleGoneOnHandPick(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	ctx := context.Background()
	if m.SettleGone(ctx, w.slotID) {
		t.Fatal("settled a download the Downloader still lists")
	}
	w.fake.drop(hash)
	if m.SettleGone(ctx, w.slotID) {
		t.Fatal("settled before any sync missed it")
	}
	missAll(w, m, 1)
	if d := w.download(t, id); d.State == "vanished" {
		t.Fatal("one miss vanished it on its own")
	}
	if !m.SettleGone(ctx, w.slotID) {
		t.Fatal("a removed download was not settled")
	}
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("download %s", d.State)
	}
	if m.SettleGone(ctx, w.slotID) {
		t.Fatal("settled twice")
	}
}

// A job moved to another category is followed, never settled as removed.
func TestSettleGoneKeepsMovedJob(t *testing.T) {
	eng := &askEngine{seedEngine: &seedEngine{jobs: []Job{{ID: hash, Category: "other", State: StateDownloading}}}}
	w, m, id := onEngine(t, "deluge", &listless{eng})
	missAll(w, m, 1)
	if m.SettleGone(context.Background(), w.slotID) {
		t.Fatal("a moved job was settled as removed")
	}
	if d := w.download(t, id); d.State == "vanished" {
		t.Fatalf("download %s", d.State)
	}
}
