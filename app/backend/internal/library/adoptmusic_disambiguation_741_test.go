package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Two artists with the same name both score 100%, so each match
// carries MusicBrainz's note that tells them apart.
func TestMusicAdoptMatchCarriesDisambiguation(t *testing.T) {
	_, s, fm, lf := musicService(t)
	fm.artists = map[string]metadata.Artist{
		"a1": {MBID: "a1", Name: "Twin", Disambiguation: "UK punk band", Status: "running"},
		"a2": {MBID: "a2", Name: "Twin", Disambiguation: "US rapper", Status: "running"},
		"a3": {MBID: "a3", Name: "Twin", Status: "running"},
	}
	writeFile(t, filepath.Join(lf.Path, "Twin", "Album (2001)", "01 - A.flac"), "audio")
	p, err := PlanMusicAdoptFrom(context.Background(), s.Read(), fm, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Folders) != 1 {
		t.Fatalf("plan %+v", p)
	}
	c := p.Folders[0]
	got := map[string]string{}
	for _, m := range append([]AdoptMatch{*c.Match}, c.Others...) {
		got[m.ID] = m.Disambiguation
	}
	want := map[string]string{"mbid:a1": "UK punk band", "mbid:a2": "US rapper", "mbid:a3": ""}
	for id, d := range want {
		if v, ok := got[id]; !ok || v != d {
			t.Errorf("%s: %q (have %v)", id, v, got)
		}
	}
}
