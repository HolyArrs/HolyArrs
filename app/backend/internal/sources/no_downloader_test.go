package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// With Sources for one protocol and no Downloader for it, a search chooses
// nothing: the Release is passed over, the Slot stays wanted and says why.
func TestSearch_NoDownloaderChoosesNothing(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	asked := ""
	chosen := 0
	sr := &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) { chosen++ },
		HasDownloader: func(_ context.Context, p string) bool { asked = p; return false }}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	got := relByName(t, f, web1080)
	sl := slotState(t, f)
	if asked != "torrent" || chosen != 0 || got.State != "passed_over" || !strings.Contains(fmt.Sprint(got.Verdict["reasons"]), "no_downloader") {
		t.Fatalf("asked %q chosen %d release %s %v", asked, chosen, got.State, got.Verdict["reasons"])
	}
	if sl.State != mediafiles.SlotWanted || sl.LastReason == nil || *sl.LastReason != "no torrent Downloader is set up" {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
	// Once a Downloader takes it, the same search chooses it.
	sr.HasDownloader = func(context.Context, string) bool { return true }
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if sl := slotState(t, f); sl.State != mediafiles.SlotChosen || chosen != 1 {
		t.Fatalf("slot %s chosen %d", sl.State, chosen)
	}
}

func TestNoDownloader_Words(t *testing.T) {
	if NoDownloader("usenet") != "no Usenet Downloader is set up" || NoDownloader("torrent") != "no torrent Downloader is set up" {
		t.Fatal(NoDownloader("usenet"), NoDownloader("torrent"))
	}
}
