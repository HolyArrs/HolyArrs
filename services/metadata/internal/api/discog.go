package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
)

// Discographies are the stored full release-group lists and album answers
// (discog.Store); their walks run as jobs on the MusicBrainz queue.
type Discographies interface {
	Artist(ctx context.Context, mbid string) (discog.Artist, bool)
	Album(ctx context.Context, mbid string) (discog.Album, bool)
	ArtistDue(discog.Artist) bool
	AlbumDue(discog.Album) bool
}

// discogWait is how long a page waits for a walk it started (P0).
var discogWait = 10 * time.Second

// discogGroup is one release group of a stored list in an answer: the
// MusicBrainz fields and, when the picture store holds it, the cover's
// record (the same as an album answer's data.pictures.cover).
type discogGroup struct {
	discog.Group
	Cover *imgstore.Info `json:"cover,omitempty"`
}

func (s *Server) discogGroups(ctx context.Context, groups []discog.Group) []discogGroup {
	out := make([]discogGroup, len(groups))
	for i, g := range groups {
		out[i].Group = g
		if s.opts.Pictures != nil {
			if c, ok := s.opts.Pictures(ctx, "release-group", g.ID)["cover"]; ok {
				out[i].Cover = &c
			}
		}
	}
	return out
}

// wantDiscog queues a walk; with wait, the channel answers when it ends.
func (s *Server) wantDiscog(ctx context.Context, key string, p int, wait bool) <-chan error {
	if s.opts.Queue == nil {
		return nil
	}
	return s.opts.Queue.Enqueue(ctx, key, p, wait)
}

// withDiscography puts an artist's stored full list into its answer:
// source_data "release-groups" (every release group, not MusicBrainz's 25
// or 100) and "release-group-count", plus data.release_groups_complete and
// data.release_group_count. With no list stored yet it queues the walk
// (P1), marks release_groups_complete false and answers partial (kept a
// minute at the edge). A stored list that is due is checked again (P3).
func (s *Server) withDiscography(mbid string, data []byte) ([]byte, bool) {
	if s.opts.Discog == nil {
		return data, false
	}
	ctx := context.Background()
	mbid = strings.ToLower(mbid)
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil || obj == nil {
		return data, false
	}
	a, ok := s.opts.Discog.Artist(ctx, mbid)
	if !ok {
		s.wantDiscog(ctx, discog.ArtistKey(mbid), fetchq.P1Finish, false)
		obj["release_groups_complete"] = json.RawMessage("false")
		out, err := json.Marshal(obj)
		if err != nil {
			return data, true
		}
		return out, true
	}
	if s.opts.Discog.ArtistDue(a) {
		s.wantDiscog(ctx, discog.ArtistKey(mbid), fetchq.P3Refresh, false)
	}
	var src map[string]json.RawMessage
	if json.Unmarshal(obj["source_data"], &src) != nil || src == nil {
		src = map[string]json.RawMessage{}
	}
	groups, err1 := json.Marshal(s.discogGroups(ctx, a.Groups))
	count, err2 := json.Marshal(a.Count)
	complete, err3 := json.Marshal(a.Complete)
	if err1 != nil || err2 != nil || err3 != nil {
		return data, false
	}
	src["release-groups"], src["release-group-count"] = groups, count
	sd, err := json.Marshal(src)
	if err != nil {
		return data, false
	}
	obj["source_data"], obj["release_groups_complete"], obj["release_group_count"] = sd, complete, count
	out, err := json.Marshal(obj)
	if err != nil {
		return data, false
	}
	return out, false
}

// storedDiscography answers the stored list for the discography route,
// starting the walk at P0 and waiting up to discogWait when none is
// stored. ok is false when there is no store, or the walk did not end in
// time (wait true then).
func (s *Server) storedDiscography(r *http.Request, mbid string) (a discog.Artist, ok, waited bool) {
	if s.opts.Discog == nil {
		return discog.Artist{}, false, false
	}
	ctx := r.Context()
	if a, ok := s.opts.Discog.Artist(ctx, mbid); ok {
		if s.opts.Discog.ArtistDue(a) {
			s.wantDiscog(context.Background(), discog.ArtistKey(mbid), fetchq.P3Refresh, false)
		}
		return a, true, false
	}
	done := s.wantDiscog(s.installCtx(r), discog.ArtistKey(mbid), fetchq.P0Waiting, true)
	if done == nil {
		return discog.Artist{}, false, false
	}
	select {
	case <-done:
	case <-time.After(discogWait):
	case <-ctx.Done():
	}
	a, ok = s.opts.Discog.Artist(ctx, mbid)
	return a, ok, !ok
}

// installCtx carries the asking install, so its P0 asks share the lane
// fairly with other installs'.
func (s *Server) installCtx(r *http.Request) context.Context {
	who, _ := r.Context().Value(callerKey{}).(caller)
	return fetchq.WithInstall(context.Background(), who.who)
}

// writeDiscography answers the discography route from the stored list.
func (s *Server) writeDiscography(w http.ResponseWriter, r *http.Request, mbid string, a discog.Artist) {
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "mbid": mbid, "source": "musicbrainz", "albums": []musicdata.DiscAlbum{},
		"release_groups": s.discogGroups(r.Context(), a.Groups), "release_group_count": a.Count, "release_groups_complete": a.Complete,
		"fetched_at": a.Walked.UTC().Format(time.RFC3339)}, 86400)
}

// serveMusicAlbum answers GET /v1/music/album/{release-group mbid}: the
// album with its chosen edition's tracks and every edition without (see
// discog.Album), from storage. Not stored yet: the walk is queued at P0
// and waited for up to discogWait, else 202 with Retry-After (X-Fetch:
// ahead queues at P4 and never waits).
func (s *Server) serveMusicAlbum(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/album/"))
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Discog == nil || s.opts.Queue == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	a, ok := s.opts.Discog.Album(r.Context(), mbid)
	if ok {
		if s.opts.Discog.AlbumDue(a) {
			s.wantDiscog(context.Background(), discog.AlbumKey(mbid), fetchq.P3Refresh, false)
		}
		s.writeAlbum(w, r, a)
		return
	}
	if s.anonSlimAbsent(r, "release-group", mbid) {
		problem(w, 404, "not_found", 0)
		return
	}
	if r.Header.Get("X-Fetch") == "ahead" {
		s.wantDiscog(context.Background(), discog.AlbumKey(mbid), fetchq.P4Deep, false)
	} else if done := s.wantDiscog(s.installCtx(r), discog.AlbumKey(mbid), fetchq.P0Waiting, true); done != nil {
		select {
		case err := <-done:
			if errors.Is(err, fetchq.ErrNotFound) {
				problem(w, 404, "not_found", 0)
				return
			}
		case <-time.After(discogWait):
		case <-r.Context().Done():
		}
		if a, ok := s.opts.Discog.Album(r.Context(), mbid); ok {
			s.writeAlbum(w, r, a)
			return
		}
	}
	w.Header().Set("Retry-After", "2")
	w.Header().Set("X-Metadata-Cache", "queued")
	writeJSON(w, 202, s.withQueueState(r.Context(), discog.AlbumKey(mbid), map[string]any{"schema_version": 1, "source": "musicbrainz", "mbid": mbid, "partial": true}))
}

func (s *Server) writeAlbum(w http.ResponseWriter, r *http.Request, a discog.Album) {
	writeMusicJSON(w, r, struct {
		SchemaVersion int    `json:"schema_version"`
		Source        string `json:"source"`
		discog.Album
	}{1, "musicbrainz", a}, 3600)
}
