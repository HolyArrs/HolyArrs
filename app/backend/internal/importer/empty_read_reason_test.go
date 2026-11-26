package importer

import (
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// #861: a finished download whose folder read found no file at all, shortly
// after it finished, waits for the files and is tried again on its own: on a
// network share the job's folder can show up a moment after the Downloader is
// done. Later, or with files read, the no-match line stays.
func TestEmptyReadWaitsForTheFiles(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	at := func(ago time.Duration) *time.Time { x := now.Add(-ago); return &x }
	one := []domain.SeenFile{{Name: "Job/Job.mkv"}}
	for _, c := range []struct {
		name   string
		files  []domain.SeenFile
		done   *time.Time
		reason string
		want   string
	}{
		{"movie, just finished, nothing read", nil, at(time.Minute), noMovieWhy, NotPresentYetWhy},
		{"show, just finished, empty list", []domain.SeenFile{}, at(9 * time.Minute), noEpisodeWhy, NotPresentYetWhy},
		{"music, just finished, nothing read", nil, at(0), noTrackWhy, NotPresentYetWhy},
		{"movie, finished long ago", nil, at(filesWait), noMovieWhy, noMovieWhy},
		{"movie, no finish time", nil, nil, noMovieWhy, noMovieWhy},
		{"movie, file read", one, at(time.Minute), noMovieWhy, noMovieWhy},
		{"show, file read", one, at(time.Minute), noEpisodeWhy, noEpisodeWhy},
		{"setup line stays", nil, at(time.Minute), "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set", "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set"},
		{"folder gone stays", nil, at(time.Minute), notThereWhy, notThereWhy},
	} {
		got := waitForFiles(domain.Download{FilesSeen: c.files, FinishedAt: c.done}, c.reason, now)
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
		if got == NotPresentYetWhy && !RetriesOnItsOwn(got) {
			t.Errorf("%s: %q is not tried again on its own", c.name, got)
		}
	}
}
