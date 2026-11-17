package downloads

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A resting Downloader's text is one sentence: a known cause is its own line
// with the fix, any other cause sits inside the sentence, not after it as a
// loose lowercase fragment.
func TestRestingDownloaderTextIsOneSentence(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	d := domain.Downloader{BackoffUntil: &later, LastError: "connection refused"}
	if got := HealthOf(d, now).Text; got != doctor.Line(doctor.Refused) {
		t.Fatalf("known cause: got %q", got)
	}
	d.LastError = "qbittorrent app/version: 502 Bad Gateway"
	want := "It did not answer, so HolyArrs waits until 12:01 UTC before sending to it again."
	if got := HealthOf(d, now).Text; got != want {
		t.Fatalf("got %q", got)
	}
	d.LastError = ""
	want = "It did not answer, so HolyArrs waits until 12:01 UTC before sending to it again."
	if got := HealthOf(d, now).Text; got != want {
		t.Fatalf("no cause: got %q", got)
	}
	d.BackoffUntil, d.State = nil, "unreachable"
	d.LastError = "read tcp 1.2.3.4:5->6.7.8.9:10: wsarecv: An existing connection was forcibly closed by the remote host."
	if got := HealthOf(d, now).Text; strings.Contains(got, "wsarecv") || strings.Contains(got, "forcibly") {
		t.Fatalf("raw error on the card: %q", got)
	}
}
