package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type householdFixture struct {
	Artists []struct {
		Name   string `json:"name"`
		MBID   string `json:"mbid"`
		Albums []struct {
			Name      string   `json:"name"`
			MBID      string   `json:"mbid"`
			Type      string   `json:"type"`
			Secondary []string `json:"secondary"`
			Days      int      `json:"days"`
			Have      bool     `json:"have"`
		} `json:"albums"`
	} `json:"artists"`
}

// loadHousehold writes testdata/household_small.json into the store and
// answers each artist's title id by name.
func loadHousehold(t *testing.T, s *database.Store, now time.Time) map[string]int64 {
	t.Helper()
	raw, err := os.ReadFile("testdata/household_small.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx householdFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(now), `{"kind":"system"}`
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		var lf, rules int64
		if err := tx.QueryRow("SELECT min(id) FROM library_folders WHERE kind = 'music'").Scan(&lf); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		for _, a := range fx.Artists {
			tid := ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, 'artist', ?, ?, ?, 'musicbrainz', 'by_hand', 'active')`, ts, by, ts, by, a.Name, a.Name, `{"musicbrainz":"`+a.MBID+`"}`)
			ids[a.Name] = tid
			var ver int64
			for _, al := range a.Albums {
				sec := "[]"
				if len(al.Secondary) > 0 {
					b, _ := json.Marshal(al.Secondary)
					sec = string(b)
				}
				day := database.FormatTime(now.AddDate(0, 0, al.Days).Truncate(24 * time.Hour))
				pid := ex(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, name, release_type, release_date, release_date_precision, source_ids, secondary_types, included)
					VALUES (?, ?, ?, ?, ?, 'album', ?, ?, ?, 'day', ?, ?, 1)`, ts, by, ts, by, tid, al.Name, al.Type, day, `{"musicbrainz":"`+al.MBID+`"}`, sec)
				if !al.Have {
					continue
				}
				if ver == 0 {
					ver = ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main) VALUES (?, ?, ?, ?, ?, 'Main', ?, ?, ?, 1)`, ts, by, ts, by, tid, rules, lf, a.Name)
				}
				item := ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind) VALUES (?, ?, ?, ?, ?, ?, 'track')`, ts, by, ts, by, tid, pid)
				slot := ex(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`, ts, by, ts, by, item, ver)
				file := ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy')`,
					ts, by, ts, by, "["+itoa(slot)+"]", ver, a.Name+"/"+al.Name+"/01.flac", ts)
				ex("UPDATE slots SET file_id = ? WHERE id = ?", file, slot)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func fakeCharts(now time.Time) (func(context.Context, string, string, string) (metadata.MusicChart, error), func(context.Context, string) (metadata.MusicGenres, error)) {
	charts := func(_ context.Context, region, kind, _ string) (metadata.MusicChart, error) {
		c := metadata.MusicChart{Region: region, Kind: kind}
		for i := 1; i <= 10; i++ {
			it := metadata.MusicChartItem{Rank: i, Title: fmt.Sprintf("Chart %s %d", kind, i), Artist: fmt.Sprintf("Charter %d", i), ArtistMBID: mid('e', i), Date: now.AddDate(0, 0, -i).Format("2006-01-02"), Genres: []string{"Pop"}, P: 1 - float64(i-1)/10}
			if kind == "albums" {
				it.MBID = mid('f', i)
				if i == 3 {
					it.MBID, it.ArtistMBID, it.Title, it.Artist, it.Explicit = apiAlbum, apiArtist, "First Light", "Lab Band", true
				}
			}
			c.Items = append(c.Items, it)
		}
		return c, nil
	}
	genres := func(_ context.Context, region string) (metadata.MusicGenres, error) {
		g := metadata.MusicGenres{Region: region}
		for i, n := range []string{"Pop", "Hip-Hop/Rap", "Country", "Rock", "Alternative", "R&B/Soul", "Latin", "Dance"} {
			g.Genres = append(g.Genres, metadata.MusicGenre{Slug: strings.ToLower(strings.NewReplacer("/", "-", "&", "and").Replace(n)), Name: n, Hue: i * 3, Count: 10 - i})
		}
		return g, nil
	}
	return charts, genres
}

// stripState is an answer with every state chip blanked and the live asks
// row left out: what layer 2 keeps.
func stripState(t *testing.T, b []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var rows []any
	for _, r := range m["rows"].([]any) {
		row := r.(map[string]any)
		if row["id"] == "your_asks" {
			continue
		}
		for _, c := range row["items"].([]any) {
			delete(c.(map[string]any), "state")
		}
		rows = append(rows, row)
	}
	m["rows"] = rows
	if h, ok := m["hero"].(map[string]any); ok {
		delete(h, "state")
	}
	if alts, ok := m["hero_alts"].([]any); ok {
		for _, c := range alts {
			delete(c.(map[string]any), "state")
		}
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func TestExploreMusicKidFirst(t *testing.T) {
	t.Parallel()
	// Music (and so every explicit card) stays off for age-limited
	// accounts, flag on or off: the Kid answer comes before the flag.
	b, err := os.ReadFile("explore_music_page.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Index(s, "h.KidMax(r)") > strings.Index(s, "h.exploreMusicPage(w, r)") {
		t.Fatal("the Kid check must run before the v2 rows")
	}
}
