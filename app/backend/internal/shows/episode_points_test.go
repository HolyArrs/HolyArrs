package shows

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The episode list scores a held file with points=1 and always carries the
// saved good-enough flag.
func TestEpisodeListPoints(t *testing.T) {
	s := open(t)
	src := &fakeShows{}
	sh := trappedShow()
	src.set(sh, "tmdb:64485")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(time.Now()), `{"kind":"system"}`
		var slot, ver int64
		if err := tx.QueryRow(`SELECT sl.id, sl.version_id FROM slots sl JOIN items i ON i.id = sl.item_id WHERE i.title_id = ? AND i.kind = 'episode' ORDER BY i.id LIMIT 1`, ti.ID).Scan(&slot, &ver); err != nil {
			return err
		}
		res, err := tx.Exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, size, added_at, quality, languages, media_info, release_name, origin, placed_by)
			VALUES (?, ?, ?, ?, '[]', ?, 'a.mkv', 1000000000, ?, '{}', '[]', '{}', 'Trapped.S01E01.1080p.WEB-DL.x264-GRP', 'imported', 'none')`, ts, by, ts, by, ver, ts)
		if err != nil {
			return err
		}
		f, _ := res.LastInsertId()
		_, err = tx.Exec(`UPDATE slots SET file_id = ?, state = 'in_library', good_enough = 0 WHERE id = ?`, f, slot)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&EpisodeHandlers{Store: s, Lib: svc.Service, Fail: func(w http.ResponseWriter, err error) { ShowError(w, err, nil) },
		Trim: func(context.Context, map[int64][]EpisodeSlot) {},
		TitleID: func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
			var x mediafiles.Title
			err := database.Get(r.Context(), s.Read(), "titles", ti.ID, &x)
			return x, err == nil
		}}).Routes(m)
	list := func(q string) []EpisodeSlot {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/titles/"+strconv.FormatInt(ti.ID, 10)+"/episodes"+q, nil))
		var body struct {
			Episodes []EpisodeView `json:"episodes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		var out []EpisodeSlot
		for _, e := range body.Episodes {
			for _, sl := range e.Slots {
				if sl.FileID != nil {
					out = append(out, sl)
				}
			}
		}
		return out
	}
	got := list("?points=1")
	if len(got) != 1 || got[0].Points == nil || got[0].GoodEnough || got[0].File != nil {
		t.Fatalf("with points: %+v", got)
	}
	if got := list(""); len(got) != 1 || got[0].Points != nil {
		t.Fatalf("without points: %+v", got)
	}
}
