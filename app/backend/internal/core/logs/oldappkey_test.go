package logs

import (
	"strings"
	"testing"
)

// The arr API (decision 0008) takes its key in the address, as Seerr
// and Overseerr send it. Every form a log line can carry it in is masked:
// the apikey query value in any case, the X-Api-Key header and the
// Authorization header with or without Bearer.
func TestOldAppKeyMasked(t *testing.T) {
	const k = "0123456789abcdef0123456789abcdef"
	l := &Log{}
	for _, in := range []string{
		"GET /sonarr/api/v3/series/lookup?term=tvdb%3A81189&apikey=" + k + " 200",
		"GET /radarr/api/v3/queue?includeMovie=true&ApiKey=" + k,
		"POST /radarr/api/v3/movie?apiKey=" + k + "&x=1",
		"X-Api-Key: " + k,
		"Authorization: Bearer " + k,
		"Authorization: " + k,
		`http: request to http://holyarrs:18730/sonarr/api/v3/system/status?apikey=` + k + ` failed`,
	} {
		if got := l.Redact(in); strings.Contains(got, k) || !strings.Contains(got, Hidden) {
			t.Errorf("Redact(%q) = %q", in, got)
		}
	}
	// The rest of the line stays readable.
	line := "GET /sonarr/api/v3/series?apikey=" + k + "&tvdbId=81189"
	if got := l.Redact(line); got != strings.Replace(line, k, Hidden, 1) {
		t.Errorf("masked line: %q", got)
	}
}
