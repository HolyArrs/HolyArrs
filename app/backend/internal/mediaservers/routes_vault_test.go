package mediaservers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Without a vault there is nowhere to keep tokens, so no route is
// registered; with one, the list answers.
func TestRoutesNeedVault(t *testing.T) {
	w := newWorld(t)
	get := func(h *Handlers) int {
		m := http.NewServeMux()
		h.Routes(m)
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/media-servers", nil))
		return rec.Code
	}
	if code := get(&Handlers{Store: w.s}); code != http.StatusNotFound {
		t.Fatalf("no vault: %d", code)
	}
	if code := get(&Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v}}); code != http.StatusOK {
		t.Fatalf("with vault: %d", code)
	}
}

// Watch turns an Event into created, modified or deleted; a delete counts
// only when files were deleted.
func TestChangeOfKinds(t *testing.T) {
	yes, no := map[string]any{"delete_files": true}, map[string]any{"delete_files": false}
	for _, c := range []struct {
		kind string
		data map[string]any
		want string
	}{
		{"import_done", nil, Created},
		{"import_partly_done", nil, Created},
		{domain.EventRenamed, nil, Modified},
		{domain.EventTitleRemoved, yes, Deleted},
		{domain.EventTitleRemoved, no, ""},
		{domain.EventTitleRemoved, nil, ""},
		{domain.EventEpisodesRemoved, yes, Deleted},
		{domain.EventEpisodesRemoved, no, ""},
		{"other", yes, ""},
		// A subtitle file written beside the video is a change in that folder.
		{"subtitle_saved", nil, subtitleWritten},
		{"subtitle_upgraded", nil, subtitleWritten},
		{"subtitle_synced", nil, subtitleWritten},
		{"subtitle_translated", nil, subtitleWritten},
		{"subtitle_cleaned", nil, subtitleWritten},
		// Searches that wrote nothing, and site pauses, tell nobody.
		{"subtitle_none_good_enough", nil, ""},
		{"subtitle_site_paused", nil, ""},
		{"subtitle_failed", nil, ""},
		{"subtitle_translate_failed", nil, ""},
		{"subtitle_turned_down", nil, ""},
		{"subtitle_held", nil, ""},
	} {
		if got := ChangeOf(domain.Event{Kind: c.kind, Data: c.data}); got != c.want {
			t.Errorf("%s %v: %q want %q", c.kind, c.data, got, c.want)
		}
	}
}
