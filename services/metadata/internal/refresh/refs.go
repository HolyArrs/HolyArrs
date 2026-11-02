package refresh

import (
	"math/rand/v2"
	"regexp"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

var rawRef = regexp.MustCompile(`^/(movie|tv|person)/([1-9][0-9]*)(/.*)?$`)

// RefOf is the upstream identity a feed can mark stale, or "" when no feed
// covers the answer (it then keeps its plain TTL). Watch providers churn
// without being flagged as changes, so they keep their TTL.
func RefOf(q model.Query) string {
	switch {
	case q.Provider == "tmdb" && q.Operation == model.OperationTMDB:
		m := rawRef.FindStringSubmatch(q.Path)
		if m == nil || m[3] == "/watch/providers" {
			return ""
		}
		return "tmdb:" + m[1] + ":" + m[2]
	case q.Provider == "tmdb" && (q.Operation == "detail" || q.Operation == "season") && q.ID != "":
		kind := "movie"
		if q.Kind == "show" {
			kind = "tv"
		}
		return "tmdb:" + kind + ":" + q.ID
	case q.Provider == "musicbrainz" && q.Operation == "detail" && q.ID != "":
		return "musicbrainz:" + q.ID
	}
	return ""
}

// TitlePage reports whether q is what a title page opens (a detail or a
// bare TMDB record), the reads the on-demand nudge applies to.
func TitlePage(q model.Query) bool {
	if q.Operation == "detail" {
		return true
	}
	if q.Operation == model.OperationTMDB {
		m := rawRef.FindStringSubmatch(q.Path)
		return m != nil && m[3] == ""
	}
	return false
}

// AgeTTL picks a verify-after by release age, with +/-10% jitter so rows
// stored together do not all come due together. A zero release counts as old.
func AgeTTL(release, now time.Time, fresh, recent, old time.Duration) time.Duration {
	ttl := old
	if !release.IsZero() {
		switch age := now.Sub(release); {
		case age < 30*24*time.Hour:
			ttl = fresh
		case age < 365*24*time.Hour:
			ttl = recent
		}
	}
	return Jitter(ttl)
}

// Jitter spreads d by +/-10%.
func Jitter(d time.Duration) time.Duration {
	return d + time.Duration((rand.Float64()*0.2-0.1)*float64(d))
}
