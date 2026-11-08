package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: GET /api/v1/events
func TestEventsFilters(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	ctx := context.Background()
	ti := addTitleFor(t, s, "Arrival")
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i, k := range []string{"title_added", "download_sent", "import_done", "renamed", "title_removed"} {
		e := &domain.Event{Kind: k, Summary: k, At: day.AddDate(0, 0, i), Data: map[string]any{"path": "/movies/Arrival (2016)"}}
		if k != "renamed" {
			e.TitleID = &ti
		}
		if err := s.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	type page struct {
		Events []struct {
			Kind      string
			TitleName string `json:"title_name"`
			Data      map[string]any
		} `json:"events"`
		Total int `json:"total"`
	}
	get := func(q string, want int) page {
		var p page
		b := lbDo(t, "GET", srv.URL+"/api/v1/events?"+q, "", want)
		if want == 200 {
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	if p := get("", 200); p.Total != 5 || p.Events[0].Kind != "title_removed" || p.Events[0].TitleName != "Arrival" || p.Events[0].Data["path"] == nil {
		t.Fatalf("all: %+v", p)
	}
	if p := get("title="+lbItoa(ti), 200); p.Total != 4 {
		t.Fatalf("by movie: %+v", p)
	}
	if p := get("kind=import_*,renamed", 200); p.Total != 2 {
		t.Fatalf("by kind: %+v", p)
	}
	// to is a whole day: 2 October 2026 includes the download sent that day.
	if p := get("from=2026-10-02&to=2026-10-03", 200); p.Total != 2 || p.Events[0].Kind != "import_done" || p.Events[1].Kind != "download_sent" {
		t.Fatalf("by dates: %+v", p)
	}
	if p := get("limit=2&offset=4", 200); p.Total != 5 || len(p.Events) != 1 || p.Events[0].Kind != "title_added" {
		t.Fatalf("paging: %+v", p)
	}
	for _, bad := range []string{"title=x", "from=yesterday", "to=13/10/2026", "offset=-1"} {
		get(bad, 400)
	}
}

// TestEveryLoopStepLeavesAnEvent is the audit for 5.1.10: each step of the
// loop names the Event kinds that record it, and each kind must be written
// by the program (not only by tests). The behaviour of each kind is tested
// in its own package; this keeps a step from silently losing its row.
func TestEveryLoopStepLeavesAnEvent(t *testing.T) {
	t.Parallel()
	steps := map[string][]string{
		"add":      {"domain.EventTitleAdded", "domain.EventAdopted"},
		"search":   {"EventSearchDone", "EventSearchFailed"},
		"hold":     {"EventReleaseHeld"},
		"choose":   {"EventReleaseChosen", "EventChosenByHand"},
		"grab":     {`"download_sent"`},
		"download": {`"download_"+d.State`},
		"fail":     {`"download_failed"`},
		"import":   {`"import_" + imp.State`, `"import_rejected"`},
		"replace":  {`"file_replaced"`},
		"rename":   {"domain.EventRenamed"},
		"delete":   {"domain.EventTitleRemoved", "domain.EventFileHeld"},
		"scan":     {"domain.EventLibraryScanned", "domain.EventLibraryRescanned", "domain.EventFileMissing"},
		"health":   {"domain.EventHealthBroken", "domain.EventHealthRecovered", "domain.EventFreeSpaceLow"},
		"decide":   {"domain.EventNeedsDecision"},
	}
	var code strings.Builder
	err := filepath.WalkDir("../../internal", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") && !strings.Contains(filepath.ToSlash(p), "/domain/") {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			code.Write(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	all := code.String()
	for step, needles := range steps {
		for _, n := range needles {
			if !strings.Contains(all, n) {
				t.Errorf("loop step %s: no code writes %s", step, n)
			}
		}
	}
}
