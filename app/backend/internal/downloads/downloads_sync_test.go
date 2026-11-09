package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// noClient is a reader Manager that never reaches a client: the store only
// holds what the test loads.
type noClient struct{ m *Manager }

func (noClient) Reader(context.Context, domain.Downloader) (Engine, uint64, error) {
	return nil, 0, errors.New("no client in this test")
}
func (noClient) ReportRead(context.Context, domain.Downloader, error) {}
func (noClient) Generation(int64) uint64                              { return 0 }
func (n noClient) Interval(a int) time.Duration                       { return n.m.Interval(a) }

// Link-only matching never adds a title: a sure match missing from the
// Library is only suggested.
func TestMatchLinkOnlyNeverAdds(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	added := 0
	x.dl.Match.AddMovie = func(context.Context, string) (int64, error) { added++; return 0, nil }
	x.dl.Match.AddShow = func(context.Context, string) (int64, error) { added++; return 0, nil }
	var titles int
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&titles)
	out, tid, err := x.dl.MatchLinkOnly(context.Background(), 1, "ABCDEF0000000000000000000000000000000000", "Some.Linux.ISO.x64", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if out != LinkNotMedia || tid != 0 {
		t.Fatalf("outcome %q %d", out, tid)
	}
	var after int
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&after)
	if added != 0 || after != titles {
		t.Fatalf("titles %d -> %d, adds %d", titles, after, added)
	}
	var st string
	if err := x.s.Read().QueryRow("SELECT state FROM client_links WHERE hash = 'abcdef0000000000000000000000000000000000'").Scan(&st); err != nil || st != LinkNotMedia {
		t.Fatalf("link %q %v", st, err)
	}
	// A sure match not in the Library: suggested, still no add.
	x.dl.Match.SearchMovies = func(context.Context, string, int) ([]metadata.Movie, error) {
		return []metadata.Movie{{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}}}, nil
	}
	out, tid, err = x.dl.MatchLinkOnly(context.Background(), 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != "suggested" || tid != 0 {
		t.Fatalf("sure match not in the Library: %q %d %v", out, tid, err)
	}
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&after)
	if added != 0 || after != titles {
		t.Fatalf("suggested match added a title: titles %d -> %d, adds %d", titles, after, added)
	}
	_ = sql.ErrNoRows
}

// downloads_auto_add on: a sure match not in the Library is added (once,
// even when matched again after a restart) and linked, with a log line;
// not-media and unsure names never add.
func TestMatchAutoAdd(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	ctx := context.Background()
	exec41(t, x.s, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'downloads_auto_add', 'true', '', '{}', '', '{}')`)
	var adds int
	var newID int64
	// The add stands in for the real one: the seeded title takes the
	// looked-up id, so the Library holds it from then on.
	_ = x.s.Read().QueryRow("SELECT id FROM titles ORDER BY id LIMIT 1").Scan(&newID)
	exec41(t, x.s, `UPDATE titles SET source_ids = '{}', kind = 'movie'`)
	x.dl.Match.AddMovie = func(ctx context.Context, id string) (int64, error) {
		adds++
		exec41(t, x.s, `UPDATE titles SET source_ids = '{"tmdb":"329865"}' WHERE id = `+fmt.Sprint(newID))
		return newID, nil
	}
	x.dl.Match.SearchMovies = func(context.Context, string, int) ([]metadata.Movie, error) {
		return []metadata.Movie{{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}}}, nil
	}
	if out, _, err := x.dl.MatchLinkOnly(ctx, 1, "cccc000000000000000000000000000000000000", "Some.Linux.ISO.x64", "linux"); err != nil || out != LinkNotMedia {
		t.Fatalf("not media: %q %v", out, err)
	}
	if out, _, err := x.dl.MatchLinkOnly(ctx, 1, "dddd000000000000000000000000000000000000", "Arival.Two.2016.1080p", "movies"); err != nil || out != LinkUnsure {
		t.Fatalf("unsure: %q %v", out, err)
	}
	if adds != 0 {
		t.Fatalf("adds %d before a sure match", adds)
	}
	out, tid, err := x.dl.MatchLinkOnly(ctx, 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != LinkLinked || tid == 0 || tid != newID || adds != 1 {
		t.Fatalf("auto add: %q %d %v adds %d", out, tid, err, adds)
	}
	var line string
	_ = x.s.Read().QueryRow("SELECT summary FROM torrent_log WHERE hash = 'bbbb000000000000000000000000000000000000'").Scan(&line)
	if line != "Added to your Library" {
		t.Fatalf("log line %q", line)
	}
	// A restart: the link row is gone (as if lost) but the title stays;
	// matching again links the same title and adds nothing.
	exec41(t, x.s, `DELETE FROM client_links`)
	out, tid2, err := x.dl.MatchLinkOnly(ctx, 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != LinkLinked || tid2 != tid || adds != 1 {
		t.Fatalf("again: %q %d %v adds %d", out, tid2, err, adds)
	}
	// Off: a new sure match is only suggested.
	exec41(t, x.s, `UPDATE settings SET value = 'false' WHERE key = 'downloads_auto_add'`)
	exec41(t, x.s, `DELETE FROM client_links`)
	exec41(t, x.s, `UPDATE titles SET source_ids = '{}'`)
	out, _, err = x.dl.MatchLinkOnly(ctx, 1, "eeee000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != "suggested" || adds != 1 {
		t.Fatalf("off: %q %v adds %d", out, err, adds)
	}
	// On again: the matcher's own suggestion is added; a person's choice
	// (any other how) stays as it is.
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, slot_ids, covers, candidates, how, matched_at) VALUES (1, 'ffff000000000000000000000000000000000000', 'Arrival.2016.1080p', 'unsure', '[]', '', '[]', 'rejected', '')`)
	exec41(t, x.s, `UPDATE settings SET value = 'true' WHERE key = 'downloads_auto_add'`)
	out, tid, err = x.dl.MatchLinkOnly(ctx, 1, "eeee000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != LinkLinked || tid != newID || adds != 2 {
		t.Fatalf("upgrade: %q %d %v adds %d", out, tid, err, adds)
	}
	out, _, err = x.dl.MatchLinkOnly(ctx, 1, "ffff000000000000000000000000000000000000", "Arrival.2016.1080p", "movies")
	if err != nil || out != LinkUnsure || adds != 2 {
		t.Fatalf("person choice changed: %q %v adds %d", out, err, adds)
	}
	// Match now with auto-add on puts suggested items back in the queue.
	exec41(t, x.s, `INSERT INTO dl_match_queue (downloader_id, key, name, category, priority, next_try_at, outcome) VALUES (1, 'gggg', 'X.2016', 'movies', 0, '', 'suggested')`)
	if n := x.dl.Rematch(ctx); n != 1 {
		t.Fatalf("rematch %d", n)
	}
	// A not-media call made by the name reading is looked at again; a
	// person's not-media pick is not.
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, slot_ids, covers, candidates, how, matched_at) VALUES (1, 'hhhh', 'Arrival_2016_Bluray-1080p', 'not_media', '[]', '', '[]', 'name', ''), (1, 'iiii', 'Arrival_2016_Bluray-1080p', 'not_media', '[]', '', '[]', 'by hand', '')`)
	if out, _, err := x.dl.MatchLinkOnly(ctx, 1, "hhhh", "Arrival_2016_Bluray-1080p", "prowlarr"); err != nil || out != LinkLinked {
		t.Fatalf("stale not media: %q %v", out, err)
	}
	if out, _, err := x.dl.MatchLinkOnly(ctx, 1, "iiii", "Arrival_2016_Bluray-1080p", "prowlarr"); err != nil || out != LinkNotMedia {
		t.Fatalf("person not media changed: %q %v", out, err)
	}
}
