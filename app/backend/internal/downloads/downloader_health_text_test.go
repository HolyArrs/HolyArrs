package downloads

import (
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A resting Downloader's text is one sentence: the cause sits inside it, not
// after it as a loose lowercase fragment.
func TestRestingDownloaderTextIsOneSentence(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	d := domain.Downloader{BackoffUntil: &later, LastError: "connection refused"}
	want := "It did not answer (connection refused), so HolyArrs waits until 12:01 UTC before sending to it again."
	if got := HealthOf(d, now).Text; got != want {
		t.Fatalf("got %q", got)
	}
	d.LastError = ""
	want = "It did not answer, so HolyArrs waits until 12:01 UTC before sending to it again."
	if got := HealthOf(d, now).Text; got != want {
		t.Fatalf("no cause: got %q", got)
	}
}
