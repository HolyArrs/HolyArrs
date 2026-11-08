package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestProbeFeed_ReadOnlyAndFindsMatch(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item("Arcane.S01E01.1080p.WEB.H264-HOLYARRS", time.Now(), 5, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	sr := &Searcher{Store: f.s}

	writes := f.s.WriteCount()
	needed, _, err := sr.ProbeFeed(context.Background(), time.Now())
	if err != nil || needed {
		t.Fatalf("unmatched feed: needed %v, error %v", needed, err)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("probe wrote %d transactions", got-writes)
	}
	summary, err := sr.Summarise(context.Background(), time.Now())
	if err != nil || summary[f.srcID].Calls != 1 {
		t.Fatalf("quiet Source calls %+v, error %v", summary[f.srcID], err)
	}
	lf.set(feedOf(item(web1080, time.Now(), 5, "")))
	needed, prepared, err := sr.ProbeFeed(context.Background(), time.Now())
	if err != nil || !needed || prepared == nil {
		t.Fatalf("matching feed: needed %v, error %v", needed, err)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("matching probe wrote %d transactions", got-writes)
	}
	summary, err = sr.Summarise(context.Background(), time.Now())
	if err != nil || summary[f.srcID].Calls != 1 {
		t.Fatalf("actionable probe double-counted quiet calls: %+v, error %v", summary[f.srcID], err)
	}
}

func TestProbeFeed_RunsJobForSourceRecovery(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item("Arcane.S01E01.1080p.WEB.H264-HOLYARRS", time.Now(), 5, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	exec(t, f, `UPDATE sources SET state = 'paused', paused_until = ?, extra = json_set(extra, '$.failures', '2')`,
		database.FormatTime(time.Now().Add(-time.Minute)))
	writes := f.s.WriteCount()
	needed, prepared, err := (&Searcher{Store: f.s}).ProbeFeed(context.Background(), time.Now())
	if err != nil || !needed || prepared == nil {
		t.Fatalf("recovery: needed %v, error %v", needed, err)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("recovery probe wrote %d transactions", got-writes)
	}
}

func runScheduledFeed(t *testing.T, f fixture, sr *Searcher, beforeStart func()) domain.Job {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	exec(t, f, "UPDATE schedules SET enabled = 0")
	exec(t, f, "UPDATE schedules SET enabled = 1, next_run = ? WHERE kind = ?", database.FormatTime(now.Add(-time.Minute)), jobs.KindFeedCheck)
	q := jobs.New(f.s, jobs.Options{Workers: 1, BaseBackoff: time.Hour, PollInterval: 5 * time.Millisecond})
	sr.Queue = q
	sr.Register(q)
	q.ProbeSchedule(jobs.KindFeedCheck, sr.ProbeFeed)
	queued, err := q.RunSchedules(ctx, now)
	if err != nil || len(queued) != 1 || queued[0] != jobs.KindFeedCheck {
		t.Fatalf("schedule: queued %v, error %v", queued, err)
	}
	if beforeStart != nil {
		beforeStart()
	}
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		js, err := database.Select[domain.Job](ctx, f.s.Read(), "jobs", "kind = ? ORDER BY id DESC LIMIT 1", jobs.KindFeedCheck)
		if err != nil {
			t.Fatal(err)
		}
		if len(js) == 1 && (js[0].State == "done" || js[0].State == "failed" || (js[0].State == "queued" && js[0].Attempt > 0)) {
			return js[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("scheduled feed did not finish")
	return domain.Job{}
}

func TestHandleFeed_PreparedAnswerScrubsReflectedKey(t *testing.T) {
	const secret = "YOUR_FAKE_SOURCE_KEY"
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = fmt.Fprintf(w, `<error code="100" description="reflected %s"/>`, secret)
	}))
	defer srv.Close()
	f := seed(t, srv.URL)
	exec(t, f, "UPDATE sources SET format = 'rss', secret_ref = 7, use_for = '{\"feed\":true}' WHERE id = ?", f.srcID)
	j := runScheduledFeed(t, f, &Searcher{Store: f.s, Secrets: fakeSecrets{7: secret}}, nil)
	b, _ := json.Marshal(j)
	if hits != 1 || string(b) == "" || strings.Contains(string(b), secret) {
		t.Fatalf("hits %d job %s", hits, b)
	}
	events, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(events)
	if strings.Contains(string(b), secret) {
		t.Fatalf("events contain Source key: %s", b)
	}
}

func TestHandleFeed_ConsumesPreparedAnswerOnce(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now(), 5, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	sr := &Searcher{Store: f.s}
	j := runScheduledFeed(t, f, sr, nil)
	if lf.hits != 1 || j.Result["releases_new"] != float64(1) {
		t.Fatalf("requests %d result %+v", lf.hits, j.Result)
	}
	summary, err := sr.Summarise(context.Background(), time.Now())
	if err != nil || summary[f.srcID].Calls != 1 {
		t.Fatalf("prepared call counted more than once: %+v, error %v", summary[f.srcID], err)
	}
}

func TestHandleFeed_DiscardsPreparedAnswerAfterSourceEdit(t *testing.T) {
	first := &liveFeed{}
	first.set(feedOf(item(web1080, time.Now(), 5, "")))
	second := &liveFeed{}
	second.set(feedOf(item("Arcane.S01E01.1080p.WEB.H264-HOLYARRS", time.Now(), 5, "")))
	f := seed(t, first.url(t))
	feedOn(t, f)
	j := runScheduledFeed(t, f, &Searcher{Store: f.s}, func() {
		exec(t, f, "UPDATE sources SET url = ?, updated_at = ? WHERE id = ?", second.url(t), database.FormatTime(time.Now()), f.srcID)
	})
	if first.hits != 1 || second.hits != 1 || j.Result["chosen"] != float64(0) || slotState(t, f).State != mediafiles.SlotWanted {
		t.Fatalf("requests first=%d second=%d result %+v", first.hits, second.hits, j.Result)
	}
}

func TestHandleFeed_RefetchesWhenWantedKindsChange(t *testing.T) {
	var mu sync.Mutex
	var cats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cats = append(cats, r.URL.Query().Get("cat"))
		mu.Unlock()
		_, _ = w.Write([]byte(feedOf(item(web1080, time.Now(), 5, ""))))
	}))
	defer srv.Close()
	f := seed(t, srv.URL)
	feedOn(t, f)
	exec(t, f, "UPDATE sources SET kinds = '[]' WHERE id = ?", f.srcID)
	runScheduledFeed(t, f, &Searcher{Store: f.s}, func() {
		exec(t, f, "UPDATE titles SET kind = 'show', updated_at = ?", database.FormatTime(time.Now()))
	})
	mu.Lock()
	defer mu.Unlock()
	if len(cats) != 2 || strings.Contains(cats[0], "5000") || !strings.Contains(cats[1], "5000") {
		t.Fatalf("request categories %v", cats)
	}
}

func TestProbeFeed_TimeoutDiscardsPartialBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	f := seed(t, srv.URL)
	feedOn(t, f)
	var base domain.Source
	if err := database.Get(context.Background(), f.s.Read(), "sources", f.srcID, &base); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for i := 1; i < feedProbeParallel+2; i++ {
			src := base
			src.ID = 0
			src.Name = fmt.Sprintf("blocked %d", i)
			if _, err := database.Insert(tx, "sources", &src, domain.System); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writes := f.s.WriteCount()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	needed, prepared, err := (&Searcher{Store: f.s}).ProbeFeed(ctx, time.Now())
	if !needed || prepared != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("needed %v prepared %T error %v", needed, prepared, err)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("timed-out probe wrote %d transactions", got-writes)
	}
}

func TestProbeFeed_NoQuietHistoryAfterSourceReuse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(feedOf(item("Arcane.S01E01.1080p.WEB.H264-HOLYARRS", time.Now(), 5, ""))))
	}))
	defer srv.Close()
	f := seed(t, srv.URL)
	feedOn(t, f)
	sr := &Searcher{Store: f.s}
	type answer struct {
		needed   bool
		prepared any
		err      error
	}
	done := make(chan answer, 1)
	go func() {
		needed, prepared, err := sr.ProbeFeed(context.Background(), time.Now())
		done <- answer{needed: needed, prepared: prepared, err: err}
	}()
	<-started
	var replacementID int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM sources WHERE id = ?", f.srcID); err != nil {
			return err
		}
		src := &domain.Source{Name: "Replacement", Protocol: "torrent", Format: "torznab", URL: srv.URL,
			UseFor: domain.SourceUse{Feed: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
			Priority: 25, State: "enabled", Extra: map[string]string{}, AllowLocal: true}
		id, err := database.Insert(tx, "sources", src, domain.System)
		replacementID = id
		return err
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	got := <-done
	if got.err != nil || !got.needed || got.prepared == nil {
		t.Fatalf("probe result needed=%v prepared=%T error=%v", got.needed, got.prepared, got.err)
	}
	sums, err := sr.Summarise(context.Background(), time.Now())
	if err != nil || sums[replacementID].Calls != 0 {
		t.Fatalf("replacement inherited quiet calls: %+v, error %v", sums[replacementID], err)
	}
}

func TestProbeGrabHeld_DuePredicateWithoutWriting(t *testing.T) {
	lf := &liveFeed{}
	f := seed(t, lf.url(t))
	sr := &Searcher{Store: f.s}
	writes := f.s.WriteCount()
	needed, _, err := sr.ProbeGrabHeld(context.Background(), time.Now())
	if err != nil || needed {
		t.Fatalf("no held release: needed %v, error %v", needed, err)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("held probe wrote %d transactions", got-writes)
	}
}
