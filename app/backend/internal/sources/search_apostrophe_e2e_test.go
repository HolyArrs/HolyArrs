package sources

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// End to end: a show whose name holds an apostrophe (straight or
// curly) is searched on a Torznab Source that finds nothing while the term
// keeps the apostrophe, gets back release names written the way sites
// write them, and one of them must be chosen. On 1.1.0 every one of them
// was refused at the title match ("no Title in the library is called ...").
func TestSearch271_ApostropheShowsChooseARelease(t *testing.T) {
	shows := []struct {
		name, base string // library name; how sites spell it
		ok         []string
	}{
		{"Schitt's Creek", "Schitts Creek", []string{
			"Schitts Creek S01E02 Pregnancy Test 1080p AMZN WEB-DL DDP5 1 H 264-Kitsune",
			"Schitts.Creek.S01E02.1080p.WEBRip.x264-TBS",
			"Schitts.Creek.S01E02.720p.WEBRip.x264-TBS[ettv]",
			"Schitt.s.Creek.S01E02.1080p.WEB.h264-TBS",
		}},
		{"Schitt’s Creek", "Schitts Creek", []string{"Schitts.Creek.S01E02.1080p.WEBRip.x264-TBS"}},
		{"Grey's Anatomy", "Greys Anatomy", []string{"Greys.Anatomy.S01E02.1080p.WEB.h264-ETHEL"}},
		{"Grey’s Anatomy", "Greys Anatomy", []string{"Greys.Anatomy.S01E02.1080p.WEB.h264-ETHEL"}},
		{"Bob's Burgers", "Bobs Burgers", []string{"Bobs.Burgers.S01E02.1080p.WEB.h264-ETHEL"}},
		{"It's Always Sunny in Philadelphia", "Its Always Sunny in Philadelphia", []string{"Its.Always.Sunny.In.Philadelphia.S01E02.1080p.WEB.h264-ETHEL"}},
		{"Marvel's Agents of S.H.I.E.L.D.", "Marvels Agents of SHIELD", []string{
			"Marvels.Agents.of.S.H.I.E.L.D.S01E02.1080p.WEB-DL.DD5.1.H.264-NTb",
			"Marvels.Agents.of.SHIELD.S01E02.720p.HDTV.x264-2HD",
		}},
		{"Shōgun", "Shogun", []string{"Shogun.S01E02.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb"}},
	}
	for _, sh := range shows {
		for _, viaID := range []bool{false, true} {
			name := sh.name
			if viaID {
				name += " (tvdb id)"
			}
			t.Run(name, func(t *testing.T) {
				for _, want := range sh.ok {
					ts := &tvServer{answer: func(q url.Values) []string {
						term := q.Get("q")
						if strings.ContainsAny(term, "'’‘`") || (term != "" && !strings.EqualFold(term, sh.base)) {
							return nil // sites find nothing for the name as written
						}
						return []string{want, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"}
					}}
					f := seedShow(t, ts.start(t), nil)
					ids := `{}`
					if viaID {
						ids = `{"tvdb":"4242"}`
					}
					if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
						_, err := tx.Exec(`UPDATE titles SET name = ?, sort_name = ?, source_ids = ? WHERE id = ?`, sh.name, strings.ToLower(sh.name), ids, f.titleID)
						return err
					}); err != nil {
						t.Fatal(err)
					}
					sr := &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) {}}
					j := runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E02"]}, nil)
					if j.State != "done" {
						t.Fatalf("job %s %+v", j.State, j.Error)
					}
					for _, q := range ts.queries {
						if strings.ContainsAny(q.Get("q"), "'’‘`") {
							t.Errorf("search term kept an apostrophe: %q", q.Get("q"))
						}
					}
					r := relByName(t, f.fixture, want)
					if r.State != "chosen" {
						t.Errorf("%q for %q: %s %v %v (queries %v)", want, sh.name, r.State, reasonCodes(r), r.Verdict["reasons"], ts.queries)
					}
					if st := f.state(t, "S01E02"); st != mediafiles.SlotChosen {
						t.Errorf("S01E02 is %s", st)
					}
				}
			})
		}
	}
}
