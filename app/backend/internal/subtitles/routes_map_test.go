package subtitles

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Each error the subtitle routes can meet maps to the status the screens
// expect, wrapped or not.
func TestSubtitleError_StatusMap(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ErrNotFound, 404},
		{fmt.Errorf("x: %w", ErrNotFound), 404},
		{ErrMusic, 409},
		{ErrNotMain, 409},
		{fmt.Errorf("x: %w", ErrNotBeside), 409},
		{ErrNotOurs, 409},
		{subtext.ErrBadFix, 409},
		{fmt.Errorf("x: %w", subtext.ErrNotSubtitle), 409},
		{ErrNeedsAccount, 409},
		{&sites.Error{}, 502},
		{fmt.Errorf("x: %w", &sites.Error{}), 502},
		{errors.New("other"), 400},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		subtitleError(rec, c.err)
		if rec.Code != c.code {
			t.Errorf("%v: %d, want %d", c.err, rec.Code, c.code)
		}
	}
}

// actorOf is the signed-in person, or the owner when nobody is signed in.
func TestActorOf_UsesSignedInUser(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	if a := actorOf(r); a.Kind != "user" || a.ID != 1 {
		t.Fatalf("no session: %+v", a)
	}
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 7}))
	if a := actorOf(r); a.Kind != "user" || a.ID != 7 {
		t.Fatalf("signed in as 7: %+v", a)
	}
}

// The job ids a search answers with are the Jobs the real queue made.
func TestSearchSubtitles_ReturnsQueuedJobIDs(t *testing.T) {
	f := newFix(t)
	s := &Service{Store: f.st}
	s.Register(jobs.New(f.st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})) // never started: jobs stay queued
	ctx := context.Background()
	if err := s.Queue(ctx, f.movieFile, nil); err != nil {
		t.Fatal(err)
	}
	h := NewHandlers(f.st, nil, s)
	ids := h.subtitleJobIDs(ctx, []int64{f.movieFile, f.ep1File})
	if len(ids) != 1 {
		t.Fatalf("job ids %v, want one (only the movie File was queued)", ids)
	}
	var kind, key string
	if err := f.st.Read().QueryRow("SELECT kind, dedupe_key FROM jobs WHERE id = ?", ids[0]).Scan(&kind, &key); err != nil {
		t.Fatal(err)
	}
	if kind != KindSearch || key != KindSearch+":"+strconv.FormatInt(f.movieFile, 10) {
		t.Fatalf("job %d: %s %s", ids[0], kind, key)
	}
}
