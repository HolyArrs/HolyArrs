package api

import (
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/control"
)

// Edge caching (music speed plan 2026-10-23, section 5 and rule 7).
//
// Shared catalogue routes carry only data that is the same for everyone
// (charts, lists, artist and album answers, pictures, the control file).
// They are public by design: they answer with or without an install
// credential, and a successful answer is cacheable by Cloudflare. Error
// answers and anything tied to an install stay no-store.

const staleIfError = ", stale-if-error=86400"

// edgePolicies maps a shared route prefix to its Cache-Control on success.
// Longest matching prefix wins (see edgePolicy).
var edgePolicies = []struct{ prefix, policy string }{
	{"/v1/control", "public, max-age=60, s-maxage=60" + staleIfError},
	{"/v1/lists/music-trending", "public, max-age=3600, s-maxage=21600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/lists/service-top", "public, max-age=3600, s-maxage=21600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/music/charts", "public, max-age=3600, s-maxage=21600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/music/pools", "public, max-age=3600, s-maxage=21600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/music/lists/shelves", "public, max-age=3600, s-maxage=3600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/music/new", "public, max-age=3600, s-maxage=21600, stale-while-revalidate=21600" + staleIfError},
	{"/v1/music/genres/tile/", "public, max-age=86400, s-maxage=604800, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/genres", "public, max-age=3600, s-maxage=604800, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/musicbrainz/artist/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/musicbrainz/release-group/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/musicbrainz/release/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/musicbrainz/recording/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	// Artist packages and their sync pages: the same for everyone, no
	// caller recorded; rebuilt hourly at most.
	{"/v1/music/packages", "public, max-age=300, s-maxage=900, stale-while-revalidate=3600" + staleIfError},
	{"/v1/music/album/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	// Popularity moves daily: an hour at the edge, apart from the artist
	// answer that keeps a day.
	{"/v1/music/popularity", "public, max-age=900, s-maxage=3600, stale-while-revalidate=3600" + staleIfError},
	{"/v1/music/similar/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/cover/", "public, max-age=86400, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/videos/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/pictures", "public, max-age=300, s-maxage=600, stale-while-revalidate=3600" + staleIfError},
	{"/v1/art/", "public, max-age=86400, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/img/", "public, max-age=31536000, immutable"},
}

// edgeAuthPolicies are shared answers the edge keeps (the same for every
// install) whose misses still need a credential: a miss may ask an outside
// source (top songs, album details) or start a MusicBrainz walk
// (discography), so an anonymous caller only ever gets the edge's copy.
var edgeAuthPolicies = []struct{ prefix, policy string }{
	{"/v1/music/top-songs/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/discography/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/music/album-details", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	// Deezer marks (kept 14 days here), an artist's Wikipedia bio (30 days)
	// and TVmaze shows: a miss asks the outside source, so it needs a
	// credential; every install then shares the edge's copy for a day.
	{"/v1/music/deezer/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/wikidata/artist/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
	{"/v1/shows/tvmaze/", "public, max-age=3600, s-maxage=86400, stale-while-revalidate=604800" + staleIfError},
}

// edgeAuthPolicy is the success Cache-Control of a kept route that needs a
// credential, "" for every other route.
func edgeAuthPolicy(path string) string {
	for _, p := range edgeAuthPolicies {
		if strings.HasPrefix(path, p.prefix) {
			return p.policy
		}
	}
	return ""
}

// partialPolicy is the success Cache-Control of an empty, partial or
// still-filling answer: a minute, never stale-while-revalidate, so the next
// ask sees the filled answer (cache audit 2026-10-23).
const partialPolicy = "public, max-age=60, s-maxage=60"

// partialHeader marks a 200 that is empty or still filling; the edge
// writer gives it partialPolicy.
const partialHeader = "X-Metadata-Partial"

// edgePolicy is the success Cache-Control of a shared route, "" for every
// other route (those keep what their handler sets).
func edgePolicy(path string) string {
	best, policy := 0, ""
	for _, p := range edgePolicies {
		if strings.HasPrefix(path, p.prefix) && len(p.prefix) > best {
			best, policy = len(p.prefix), p.policy
		}
	}
	return policy
}

// edgeWriter rewrites the headers of a shared route: success answers get
// the public policy (a handler that already set a public policy keeps it)
// and lose Vary: Authorization; every other status is no-store.
type edgeWriter struct {
	http.ResponseWriter
	policy string
	wrote  bool
}

func (e *edgeWriter) WriteHeader(code int) {
	if !e.wrote {
		e.wrote = true
		h := e.Header()
		switch {
		case (code == 200 || code == 304) && h.Get("Vary") == "Accept" && strings.HasPrefix(h.Get("Cache-Control"), "private"):
			// Chosen by Accept: the handler's browser-only policy stays,
			// partial or not, so the edge never keeps one format for all.
		case (code == 200 || code == 304 || code == 301) && h.Get(partialHeader) != "":
			h.Set("Cache-Control", partialPolicy)
			h.Del("Vary")
		case (code == 200 || code == 304 || code == 301) && h.Get("X-Metadata-Placeholder") != "":
			// A stand-in picture is never kept.
			h.Set("Cache-Control", "no-store")
			h.Del("Vary")
		case code == 200 || code == 304 || code == 301:
			if !strings.HasPrefix(h.Get("Cache-Control"), "public") {
				h.Set("Cache-Control", e.policy)
			}
			if strings.EqualFold(h.Get("Vary"), "Authorization") {
				h.Del("Vary")
			}
		case code == 302 && strings.HasPrefix(h.Get("Cache-Control"), "public"):
			// A short-lived redirect the handler chose (pictures not stored yet).
		default:
			h.Set("Cache-Control", "no-store")
			h.Del("Vary")
		}
	}
	e.ResponseWriter.WriteHeader(code)
}

func (e *edgeWriter) Write(b []byte) (int, error) {
	if !e.wrote {
		e.WriteHeader(200)
	}
	return e.ResponseWriter.Write(b)
}

// serveControl answers the kill switch JSON (defaults when unset).
func (s *Server) serveControl(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	sw := s.opts.Control
	if sw == nil {
		sw = control.New("")
	}
	body, etag := sw.Body()
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(304)
		return
	}
	w.WriteHeader(200)
	_, _ = w.Write(body)
}

// EdgePrefixes lists every shared route prefix the edge may keep; the
// purge command clears each one after a deploy.
func EdgePrefixes() []string {
	out := make([]string, 0, len(edgePolicies)+len(edgeAuthPolicies))
	for _, p := range edgePolicies {
		out = append(out, p.prefix)
	}
	for _, p := range edgeAuthPolicies {
		out = append(out, p.prefix)
	}
	return out
}
