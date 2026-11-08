package requests

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestEpisodeWords(t *testing.T) {
	if got := EpisodeWords(1, "Slow Horses"); got != "A new episode of Slow Horses" {
		t.Fatal(got)
	}
	if got := EpisodeWords(3, "Slow Horses"); got != "3 new episodes of Slow Horses" {
		t.Fatal(got)
	}
}

func TestNewEpisodesBatchedPerShowPerHour(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{1}); err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// bo turns new-episode notices off; ana keeps the default (on).
	off := false
	if err := SaveTargets(ctx, r.st, r.bo.UserID, Targets{NewEpisodes: &off}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadTargets(ctx, r.st.Read(), r.ana.UserID); got.NewEpisodes == nil || !*got.NewEpisodes {
		t.Fatalf("ana default %+v", got)
	}
	if got, _ := ReadTargets(ctx, r.st.Read(), r.bo.UserID); got.NewEpisodes == nil || *got.NewEpisodes {
		t.Fatalf("bo off %+v", got)
	}
	var slots []int64
	rows, err := r.st.Read().Query(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND i.kind = 'episode' ORDER BY s.id`, *a.TitleID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		slots = append(slots, id)
	}
	rows.Close()
	if len(slots) < 3 {
		t.Fatalf("slots %v", slots)
	}
	r.svc.WatchEpisodes()
	arrive := func(ids ...int64) {
		if err := r.st.AddEvent(ctx, &domain.Event{Kind: "import_done", TitleID: a.TitleID, Data: map[string]any{"placed_slot_ids": ids}}); err != nil {
			t.Fatal(err)
		}
		r.svc.Wait()
	}
	arrive(slots[0], slots[1])
	r.clock = r.clock.Add(20 * time.Minute)
	arrive(slots[2])
	arrive(slots[0]) // an upgrade of a noted episode is not new
	before := len(r.fake.moments())

	// Not an hour since the first one yet: nothing is told.
	r.clock = r.clock.Add(30 * time.Minute)
	if n, err := r.svc.SendEpisodeNotices(ctx); err != nil || n != 0 {
		t.Fatalf("early: %d %v", n, err)
	}
	r.clock = r.clock.Add(11 * time.Minute)
	if n, err := r.svc.SendEpisodeNotices(ctx); err != nil || n != 1 {
		t.Fatalf("due: %d %v", n, err)
	}
	r.svc.Wait()
	if len(r.fake.moments()) != before+1 {
		t.Fatalf("alerts %v", r.fake.moments())
	}
	m := r.fake.last()
	if m.Moment != alerts.MomentNewEpisodes || m.Subject != "3 new episodes of Lab Show" || len(m.People) != 1 || m.People[0].Name != "ana" {
		t.Fatalf("alert %+v", m)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE kind = 'new_episodes' AND user_id = ? AND text = '3 new episodes of Lab Show.' AND title_id = ?", r.ana.UserID, *a.TitleID).Scan(&n)
	if n != 1 {
		t.Fatalf("ana's messages %d", n)
	}
	_ = r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE kind = 'new_episodes' AND user_id = ?", r.bo.UserID).Scan(&n)
	if n != 0 {
		t.Fatalf("bo opted out but got %d", n)
	}
	// Sent once: running again tells no one.
	if n, err := r.svc.SendEpisodeNotices(ctx); err != nil || n != 0 {
		t.Fatalf("again: %d %v", n, err)
	}
}
