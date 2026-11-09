package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A download waiting for its files (a share mounted late) says so on its
// row, not that someone paused it (late-mounts-3). One a person paused
// keeps the plain paused line.
func TestPausedRowShowsWaitingReason(t *testing.T) {
	t.Parallel()
	reason := "the Downloader can't use the files (missingFiles): is the share mounted?"
	v := downloads.DownloadViewOf(domain.Download{State: "paused", StateReason: &reason})
	if !strings.HasPrefix(v.Text, "The Downloader can't use the files") || strings.Contains(v.Text, "Someone paused") {
		t.Fatalf("waiting row text %q", v.Text)
	}
	if v := downloads.DownloadViewOf(domain.Download{State: "paused"}); !strings.Contains(v.Text, "Someone paused") {
		t.Fatalf("paused by a person: %q", v.Text)
	}

	srv, s := rqSeedDownload(t, mediafiles.SlotDownloading, "paused")
	rqExec(t, s, "UPDATE downloads SET state_reason = ?", reason)
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(rqDo(t, "GET", srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 1 || !strings.HasPrefix(out.Downloads[0].Text, "The Downloader can't use the files") {
		t.Fatalf("Downloads row %+v", out.Downloads)
	}
}
