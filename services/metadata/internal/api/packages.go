package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musicpkg"
)

// Packages are the stored artist packages (musicpkg.Store).
type Packages interface {
	Get(ctx context.Context, mbid string) ([]byte, bool)
	Page(ctx context.Context, since int64, limit, maxBytes int) ([]json.RawMessage, int64, bool, error)
	Index(ctx context.Context, since int64, limit int) ([]musicpkg.IndexEntry, int64, bool, error)
	Latest(ctx context.Context) int64
}

// Sync page sizes.
const (
	packagesLimit     = 50
	packagesMaxLimit  = 200
	packagesIndexMax  = 5000
	packagesPageBytes = 8 << 20
)

// servePackage answers GET /v1/music/packages/{mbid}: one stored package,
// or 404 when the artist is not covered (nothing is queued; an app server
// asks for that at /v1/music/package-want/{mbid}).
func (s *Server) servePackage(w http.ResponseWriter, r *http.Request, mbid string) {
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	body, ok := s.opts.Packages.Get(r.Context(), mbid)
	if !ok {
		problem(w, 404, "not_covered", 0)
		return
	}
	writeRawMusicJSON(w, r, body)
}

// servePackages answers GET /v1/music/packages?since=<cursor>&limit=<n>
// (full packages changed after the cursor, oldest change first) and with
// view=index the same walk as id, version and hash lines only. No caller
// is recorded: the answer is the same for everyone and kept at the edge.
func (s *Server) servePackages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for k, v := range q {
		if len(v) != 1 || (k != "since" && k != "limit" && k != "view") {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	since := int64(0)
	if v := q.Get("since"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			problem(w, 400, "invalid_query", 0)
			return
		}
		since = n
	}
	index := false
	switch q.Get("view") {
	case "":
	case "index":
		index = true
	default:
		problem(w, 400, "invalid_query", 0)
		return
	}
	limit, max := packagesLimit, packagesMaxLimit
	if index {
		limit, max = packagesIndexMax, packagesIndexMax
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > max {
			problem(w, 400, "invalid_query", 0)
			return
		}
		limit = n
	}
	ctx := r.Context()
	latest := s.opts.Packages.Latest(ctx)
	if index {
		lines, next, more, err := s.opts.Packages.Index(ctx, since, limit)
		if err != nil {
			problem(w, 503, "storage_unavailable", 0)
			return
		}
		writeMusicJSON(w, r, map[string]any{"schema_version": musicpkg.SchemaVersion, "since": since, "next": next, "more": more,
			"latest": latest, "packages": lines}, 300)
		return
	}
	bodies, next, more, err := s.opts.Packages.Page(ctx, since, limit, packagesPageBytes)
	if err != nil {
		problem(w, 503, "storage_unavailable", 0)
		return
	}
	if bodies == nil {
		bodies = []json.RawMessage{}
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": musicpkg.SchemaVersion, "since": since, "next": next, "more": more,
		"latest": latest, "packages": bodies}, 300)
}

// Ask quotas a credential: a minute's rate and burst.
var (
	wantPerMinute    = 1.0
	wantBurst        = 30
	wantRegPerMinute = 0.1
	wantRegBurst     = 5
)

// servePackageWant answers GET /v1/music/package-want/{mbid} (credential
// needed, never kept): an app server asks for an uncovered artist to be
// covered. Only the id and the time are kept; who asked is not.
func (s *Server) servePackageWant(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/package-want/"))
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Packages == nil || s.opts.WantPackage == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	if _, ok := s.opts.Packages.Get(r.Context(), mbid); ok {
		writeJSON(w, 200, map[string]any{"schema_version": musicpkg.SchemaVersion, "mbid": mbid, "state": "covered"})
		return
	}
	// Per-credential quota, in memory only (never stored): self-registered
	// credentials get a small one.
	who, _ := r.Context().Value(callerKey{}).(caller)
	rate, burst := wantPerMinute, wantBurst
	if who.registered {
		rate, burst = wantRegPerMinute, wantRegBurst
	}
	// The asking address's group has a bucket of its own too, so many
	// credentials from one place do not multiply the asks.
	if !s.allowBucket("want|"+who.who, rate, burst) || !s.allowBucket("want|ip|"+addressGroup(s.clientAddress(r)), rate, burst) {
		problem(w, 429, "want_limit", time.Minute)
		return
	}
	if err := s.opts.WantPackage(context.Background(), mbid); err != nil {
		problem(w, 503, "storage_unavailable", 0)
		return
	}
	writeJSON(w, 202, map[string]any{"schema_version": musicpkg.SchemaVersion, "mbid": mbid, "state": "queued"})
}

// writeRawMusicJSON answers stored JSON as writeMusicJSON does.
func writeRawMusicJSON(w http.ResponseWriter, r *http.Request, body []byte) {
	writeMusicJSON(w, r, json.RawMessage(body), 3600)
}
