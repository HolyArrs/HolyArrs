package mbread

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	_ "modernc.org/sqlite"
)

const (
	artistGID = "11111111-1111-1111-1111-111111111111"
	guestGID  = "22222222-2222-2222-2222-222222222222"
	groupGID  = "33333333-3333-3333-3333-333333333333"
	singleGID = "44444444-4444-4444-4444-444444444444"
	relA      = "55555555-5555-5555-5555-555555555555" // official, 2001 GB
	relB      = "66666666-6666-6666-6666-666666666666" // official, 1999 US, 2 discs
	relC      = "77777777-7777-7777-7777-777777777777" // bootleg, 1998
	recGID    = "88888888-8888-8888-8888-888888888888"
	missing   = "99999999-9999-9999-9999-999999999999"
)

// fakeDump is a small copy with the slim tables' columns the reader uses.
var fakeDump = `
CREATE TABLE artist(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, sort_name TEXT, begin_date_year INTEGER, begin_date_month INTEGER, begin_date_day INTEGER,
 end_date_year INTEGER, end_date_month INTEGER, end_date_day INTEGER, type INTEGER, area INTEGER, gender INTEGER, comment TEXT, edits_pending INTEGER,
 last_updated TEXT, ended TEXT, begin_area INTEGER, end_area INTEGER);
CREATE TABLE artist_type(id INTEGER PRIMARY KEY, name TEXT, parent INTEGER, child_order INTEGER, description TEXT, gid TEXT);
CREATE TABLE artist_alias(id INTEGER PRIMARY KEY, artist INTEGER, name TEXT, locale TEXT, edits_pending INTEGER, last_updated TEXT, type INTEGER, sort_name TEXT,
 begin_date_year INTEGER, begin_date_month INTEGER, begin_date_day INTEGER, end_date_year INTEGER, end_date_month INTEGER, end_date_day INTEGER, primary_for_locale TEXT, ended TEXT);
CREATE TABLE artist_credit(id INTEGER PRIMARY KEY, name TEXT, artist_count INTEGER, ref_count INTEGER, created TEXT, edits_pending INTEGER, gid TEXT);
CREATE TABLE artist_credit_name(artist_credit INTEGER, position INTEGER, artist INTEGER, name TEXT, join_phrase TEXT);
CREATE TABLE area(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, type INTEGER, edits_pending INTEGER, last_updated TEXT, begin_date_year INTEGER,
 begin_date_month INTEGER, begin_date_day INTEGER, end_date_year INTEGER, end_date_month INTEGER, end_date_day INTEGER, ended TEXT, comment TEXT);
CREATE TABLE iso_3166_1(area INTEGER, code TEXT);
CREATE TABLE release_group(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, artist_credit INTEGER, type INTEGER, comment TEXT, edits_pending INTEGER, last_updated TEXT);
CREATE TABLE release_group_primary_type(id INTEGER PRIMARY KEY, name TEXT, parent INTEGER, child_order INTEGER, description TEXT, gid TEXT);
CREATE TABLE release_group_secondary_type(id INTEGER PRIMARY KEY, name TEXT, parent INTEGER, child_order INTEGER, description TEXT, gid TEXT);
CREATE TABLE release_group_secondary_type_join(release_group INTEGER, secondary_type INTEGER, created TEXT);
CREATE TABLE release(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, artist_credit INTEGER, release_group INTEGER, status INTEGER, packaging INTEGER,
 language INTEGER, script INTEGER, barcode TEXT, comment TEXT, edits_pending INTEGER, quality INTEGER, last_updated TEXT);
CREATE TABLE release_status(id INTEGER PRIMARY KEY, name TEXT, parent INTEGER, child_order INTEGER, description TEXT, gid TEXT);
CREATE TABLE release_country(release INTEGER, country INTEGER, date_year INTEGER, date_month INTEGER, date_day INTEGER);
CREATE TABLE release_unknown_country(release INTEGER, date_year INTEGER, date_month INTEGER, date_day INTEGER);
CREATE TABLE medium(id INTEGER PRIMARY KEY, release INTEGER, position INTEGER, format INTEGER, name TEXT, edits_pending INTEGER, last_updated TEXT, track_count INTEGER, gid TEXT);
CREATE TABLE medium_format(id INTEGER PRIMARY KEY, name TEXT, parent INTEGER, child_order INTEGER, year INTEGER, has_discids TEXT, description TEXT, gid TEXT);
CREATE TABLE track(id INTEGER PRIMARY KEY, gid TEXT, recording INTEGER, medium INTEGER, position INTEGER, number TEXT, name TEXT, artist_credit INTEGER,
 length INTEGER, edits_pending INTEGER, last_updated TEXT, is_data_track TEXT);
CREATE TABLE recording(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, artist_credit INTEGER, length INTEGER, comment TEXT, edits_pending INTEGER, last_updated TEXT, video TEXT);
CREATE TABLE url(id INTEGER PRIMARY KEY, gid TEXT, url TEXT, edits_pending INTEGER, last_updated TEXT);
CREATE TABLE link(id INTEGER PRIMARY KEY, link_type INTEGER, begin_date_year INTEGER, begin_date_month INTEGER, begin_date_day INTEGER,
 end_date_year INTEGER, end_date_month INTEGER, end_date_day INTEGER, attribute_count INTEGER, created TEXT, ended TEXT);
CREATE TABLE link_type(id INTEGER PRIMARY KEY, parent INTEGER, child_order INTEGER, gid TEXT, entity_type0 TEXT, entity_type1 TEXT, name TEXT);
CREATE TABLE l_artist_url(id INTEGER PRIMARY KEY, link INTEGER, entity0 INTEGER, entity1 INTEGER);
CREATE TABLE l_release_group_url(id INTEGER PRIMARY KEY, link INTEGER, entity0 INTEGER, entity1 INTEGER);
CREATE TABLE l_release_url(id INTEGER PRIMARY KEY, link INTEGER, entity0 INTEGER, entity1 INTEGER);
CREATE TABLE genre(id INTEGER PRIMARY KEY, gid TEXT, name TEXT, comment TEXT, edits_pending INTEGER, last_updated TEXT);
CREATE TABLE tag(id INTEGER PRIMARY KEY, name TEXT, ref_count INTEGER);
CREATE TABLE artist_tag(artist INTEGER, tag INTEGER, count INTEGER, last_updated TEXT);
CREATE TABLE release_group_tag(release_group INTEGER, tag INTEGER, count INTEGER, last_updated TEXT);

INSERT INTO area VALUES(221,'8a754a16-0027-3a29-b6d7-2b40ea0481ed','United Kingdom',1,0,'',NULL,NULL,NULL,NULL,NULL,NULL,'f','');
INSERT INTO area VALUES(222,'489ce91b-6658-3307-9877-795b68554c98','United States',1,0,'',NULL,NULL,NULL,NULL,NULL,NULL,'f','');
INSERT INTO iso_3166_1 VALUES(221,'GB'),(222,'US');
INSERT INTO artist_type VALUES(2,'Group',NULL,0,'','e431f5f6-b5d2-343d-8b36-72607fffb74b');
INSERT INTO artist VALUES(1,'` + artistGID + `','Test Band','Band, Test',1991,NULL,NULL,NULL,NULL,NULL,2,221,NULL,'uk rock',0,'', 'f',221,NULL);
INSERT INTO artist VALUES(2,'` + guestGID + `','Guest','Guest',NULL,NULL,NULL,NULL,NULL,NULL,NULL,222,1,'',0,'','f',NULL,NULL);
INSERT INTO artist_alias VALUES(1,1,'TB','en',0,'',1,'TB',NULL,NULL,NULL,NULL,NULL,NULL,'t','f');
INSERT INTO artist_credit VALUES(10,'Test Band',1,1,'',0,''),(11,'Test Band feat. Guest',2,1,'',0,'');
INSERT INTO artist_credit_name VALUES(10,0,1,'Test Band',''),(11,0,1,'Test Band',' feat. '),(11,1,2,'Guest','');
INSERT INTO release_group_primary_type VALUES(1,'Album',NULL,1,'','f529b476-6e62-324f-b0aa-1f3e33d313fc'),(2,'Single',NULL,2,'','d6038452-8ee0-3f68-affc-2de9a1ede0b9');
INSERT INTO release_group_secondary_type VALUES(6,'Live',NULL,0,'','6fd474e2-6b58-3102-9d17-d6f7eb7da0a0');
INSERT INTO release_group VALUES(100,'` + groupGID + `','First Album',10,1,'',0,'');
INSERT INTO release_group VALUES(101,'` + singleGID + `','Live Single',11,2,'',0,'');
INSERT INTO release_group_secondary_type_join VALUES(101,6,'');
INSERT INTO release_status VALUES(1,'Official',NULL,1,'','4e304316-386d-3409-af2e-78857eec5cfe'),(3,'Bootleg',NULL,3,'','1156806e-d06a-38bd-83f0-cf2284a808b9');
INSERT INTO release VALUES(200,'` + relA + `','First Album',10,100,1,NULL,NULL,NULL,'5012345678900','',0,1,'');
INSERT INTO release VALUES(201,'` + relB + `','First Album',10,100,1,NULL,NULL,NULL,'0123456789012','US edition',0,1,'');
INSERT INTO release VALUES(202,'` + relC + `','First Album (live tape)',10,100,3,NULL,NULL,NULL,'','',0,1,'');
INSERT INTO release_country VALUES(200,221,2001,3,5),(201,222,1999,10,NULL);
INSERT INTO release_unknown_country VALUES(202,1998,NULL,NULL);
INSERT INTO medium_format VALUES(1,'CD',NULL,0,1982,'t','',''),(12,'Digital Media',NULL,0,NULL,'f','','');
INSERT INTO medium VALUES(300,200,1,1,'',0,'',2,''),(301,201,1,1,'',0,'',1,''),(302,201,2,1,'Bonus',0,'',1,''),(303,202,1,NULL,'',0,'',1,'');
INSERT INTO recording VALUES(400,'` + recGID + `','Song One',10,201000,'',0,'','f'),(401,'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','Song Two',11,180000,'',0,'','f');
INSERT INTO track VALUES(500,'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb0',400,300,1,'1','Song One',10,201000,0,'','f');
INSERT INTO track VALUES(501,'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb1',401,300,2,'2','Song Two',11,NULL,0,'','f');
INSERT INTO track VALUES(502,'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb2',400,301,1,'1','Song One',10,201000,0,'','f');
INSERT INTO track VALUES(503,'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb3',401,302,1,'1','Song Two',11,180000,0,'','f');
INSERT INTO track VALUES(504,'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb4',400,303,1,'1','Song One (live)',10,NULL,0,'','f');
INSERT INTO url VALUES(600,'cccccccc-cccc-cccc-cccc-cccccccccccc','https://www.wikidata.org/wiki/Q1',0,''),(601,'dddddddd-dddd-dddd-dddd-dddddddddddd','https://testband.example',0,'');
INSERT INTO link_type VALUES(352,NULL,0,'689870a4-a1e4-4912-b17f-7b2664215698','artist','url','wikidata'),(183,NULL,0,'fe33d22f-c3b0-4d68-bd53-a856badf2b15','artist','url','official homepage');
INSERT INTO link VALUES(700,352,NULL,NULL,NULL,NULL,NULL,NULL,0,'','f'),(701,183,NULL,NULL,NULL,NULL,NULL,NULL,0,'','f');
INSERT INTO l_artist_url VALUES(800,700,1,600),(801,701,1,601);
INSERT INTO l_release_group_url VALUES(802,700,100,600);
INSERT INTO genre VALUES(900,'0e3fc579-2d24-4f20-9dae-736e1ec78798','rock','',0,'');
INSERT INTO tag VALUES(950,'rock',10),(951,'seen live',3);
INSERT INTO artist_tag VALUES(1,950,5,''),(1,951,1,'');
INSERT INTO release_group_tag VALUES(100,950,2,'');
`

// writeCopy makes root/<date>/mb.sqlite (title marks the copy).
func writeCopy(t testing.TB, root, date, title string) {
	t.Helper()
	dir := filepath.Join(root, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, mbdump.DBName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fakeDump); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE release_group SET name=? WHERE id=100`, title); err != nil {
		t.Fatal(err)
	}
}

func point(t testing.TB, root, date string) {
	t.Helper()
	tmp := filepath.Join(root, "CURRENT.tmp")
	if err := os.WriteFile(tmp, []byte(date+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var err error
	for i := 0; i < 50; i++ {
		// Windows refuses a rename over a file a reader has open for a
		// moment; Linux (the server) renames atomically.
		if err = os.Rename(tmp, filepath.Join(root, "CURRENT")); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(err)
}

func open(t testing.TB) *Reader {
	t.Helper()
	root := t.TempDir()
	writeCopy(t, root, "20261025-001001", "First Album")
	point(t, root, "20261025-001001")
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// liveArtist is a live MusicBrainz artist detail answer as the server
// stores it (lookup with aliases+url-rels+genres+tags, then the
// release-group browse merged in), trimmed to one of each list item.
const liveArtist = `{"source":"musicbrainz","source_id":"x","kind":"artist","title":"Test Band","summary":"uk rock","source_data":{
"id":"x","name":"Test Band","sort-name":"Band, Test","disambiguation":"uk rock","type":"Group","gender":null,"country":"GB",
"area":{"id":"a","name":"United Kingdom","sort-name":"United Kingdom","disambiguation":"","iso-3166-1-codes":["GB"]},
"begin-area":{"id":"a","name":"United Kingdom","sort-name":"United Kingdom","disambiguation":"","iso-3166-1-codes":["GB"]},
"end-area":null,"life-span":{"begin":"1991","end":null,"ended":false},"ipis":[],"isnis":[],
"aliases":[{"name":"TB","sort-name":"TB","locale":"en","primary":true,"ended":false,"type":null,"begin":null,"end":null}],
"tags":[{"name":"rock","count":5}],"genres":[{"id":"g","name":"rock","count":5,"disambiguation":""}],
"relations":[{"type":"wikidata","type-id":"t","target-type":"url","direction":"forward","ended":false,"begin":null,"end":null,
 "attributes":[],"attribute-values":{},"attribute-ids":{},"source-credit":"","target-credit":"","url":{"id":"u","resource":"https://www.wikidata.org/wiki/Q1"}}],
"release-groups":[{"id":"g","title":"First Album","disambiguation":"","primary-type":"Album","primary-type-id":"p","secondary-types":[],
 "secondary-type-ids":[],"first-release-date":"1998"}],"release-group-count":2}}`

const liveGroup = `{"source":"musicbrainz","source_id":"x","kind":"release-group","title":"First Album","summary":"","source_data":{
"id":"x","title":"First Album","disambiguation":"","primary-type":"Album","primary-type-id":"p","secondary-types":[],"secondary-type-ids":[],
"first-release-date":"1998",
"artist-credit":[{"name":"Test Band","joinphrase":"","artist":{"id":"a","name":"Test Band","sort-name":"Band, Test","disambiguation":"uk rock","type":"Group"}}],
"releases":[{"id":"r","title":"First Album","status":"Official","status-id":"s","date":"1999-10","country":"US","barcode":"0123456789012",
 "disambiguation":"US edition","packaging":null,"quality":"normal","release-events":[{"date":"1999-10","area":{"id":"a","name":"United States",
 "sort-name":"United States","disambiguation":"","iso-3166-1-codes":["US"]}}],"label-info":[]}],
"tags":[{"name":"rock","count":2}],"genres":[{"id":"g","name":"rock","count":2,"disambiguation":""}],
"relations":[{"type":"wikidata","type-id":"t","target-type":"url","direction":"forward","ended":false,"begin":null,"end":null,
 "attributes":[],"attribute-values":{},"attribute-ids":{},"source-credit":"","target-credit":"","url":{"id":"u","resource":"https://www.wikidata.org/wiki/Q1"}}],
"media":[{"position":1,"format":"CD","title":"","track-count":1,"track-offset":0,"tracks":[{"id":"t","position":1,"number":"1","title":"Song One",
 "length":201000,"artist-credit":[{"name":"Test Band","joinphrase":"","artist":{"id":"a","name":"Test Band","sort-name":"Band, Test","disambiguation":"uk rock","type":"Group"}}],
 "recording":{"id":"r","title":"Song One","length":201000,"disambiguation":"","video":false,
 "artist-credit":[{"name":"Test Band","joinphrase":"","artist":{"id":"a","name":"Test Band","sort-name":"Band, Test","disambiguation":"uk rock","type":"Group"}}]}}]}],
"tracklist_release":"r"}}`

// sameShape reports every key path of want missing from got or holding
// another JSON type (arrays compared by their first item).
func sameShape(path string, want, got any) []string {
	var out []string
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{path + ": want object"}
		}
		for k, v := range w {
			gv, ok := g[k]
			if !ok {
				out = append(out, path+"."+k+": missing")
				continue
			}
			if v == nil || gv == nil {
				continue // nullable either way
			}
			out = append(out, sameShape(path+"."+k, v, gv)...)
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{path + ": want array"}
		}
		if len(w) > 0 && len(g) > 0 {
			out = append(out, sameShape(path+"[0]", w[0], g[0])...)
		}
	default:
		if fmt.Sprintf("%T", want) != fmt.Sprintf("%T", got) {
			out = append(out, fmt.Sprintf("%s: want %T, got %T", path, want, got))
		}
	}
	return out
}

func TestShapeParity(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	for _, c := range []struct{ kind, id, live string }{{"artist", artistGID, liveArtist}, {"release-group", groupGID, liveGroup}} {
		b, date, ok := r.Stored(ctx, c.kind, c.id)
		if !ok || date != "20261025-001001" {
			t.Fatalf("%s: ok=%v date=%q", c.kind, ok, date)
		}
		var want, got map[string]any
		if err := json.Unmarshal([]byte(c.live), &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if diff := sameShape(c.kind, want, got); len(diff) > 0 {
			t.Errorf("%s shape differs from the live answer:\n%v", c.kind, diff)
		}
		tag, _ := got["source_tag"].(map[string]any)
		if tag["source"] != "mb-dump" || tag["ref"] != "20261025-001001" {
			t.Errorf("%s: source tag %v", c.kind, tag)
		}
	}
}

func TestArtistValues(t *testing.T) {
	r := open(t)
	b, ok := r.StoredData(context.Background(), "artist", artistGID)
	if !ok {
		t.Fatal("artist missing")
	}
	var d struct {
		Title  string `json:"title"`
		Source struct {
			Country   string `json:"country"`
			Count     int    `json:"release-group-count"`
			Relations []struct {
				URL struct {
					Resource string `json:"resource"`
				} `json:"url"`
			} `json:"relations"`
			Groups []struct {
				ID, Title string
				Primary   string   `json:"primary-type"`
				Secondary []string `json:"secondary-types"`
				First     string   `json:"first-release-date"`
			} `json:"release-groups"`
			Genres []struct{ Name string } `json:"genres"`
			Tags   []struct{ Name string } `json:"tags"`
		} `json:"source_data"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	s := d.Source
	if d.Title != "Test Band" || s.Country != "GB" || s.Count != 2 || len(s.Groups) != 2 || len(s.Relations) != 2 || len(s.Genres) != 1 || len(s.Tags) != 2 {
		t.Fatalf("artist answer: %+v", d)
	}
	// Oldest first; the album's first date is its earliest release event.
	if s.Groups[0].ID != groupGID || s.Groups[0].First != "1998" || s.Groups[0].Primary != "Album" {
		t.Errorf("first group %+v", s.Groups[0])
	}
	if s.Groups[1].ID != singleGID || len(s.Groups[1].Secondary) != 1 || s.Groups[1].Secondary[0] != "Live" {
		t.Errorf("guest single %+v", s.Groups[1])
	}
}

func TestReleaseGroupTracklist(t *testing.T) {
	r := open(t)
	b, ok := r.StoredData(context.Background(), "release-group", groupGID)
	if !ok {
		t.Fatal("group missing")
	}
	var d struct {
		Source struct {
			Tracks   string `json:"tracklist_release"`
			Releases []struct {
				ID, Status, Barcode, Date, Country string
			} `json:"releases"`
			Media []struct {
				Count  int `json:"track-count"`
				Tracks []struct {
					Title string
				} `json:"tracks"`
			} `json:"media"`
		} `json:"source_data"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	// Earliest official release: relB (1999, US), not the 1998 bootleg.
	if d.Source.Tracks != relB || len(d.Source.Releases) != 3 || len(d.Source.Media) != 2 || len(d.Source.Media[1].Tracks) != 1 {
		t.Fatalf("group answer %+v", d.Source)
	}
	for _, rel := range d.Source.Releases {
		if rel.ID == relB && (rel.Date != "1999-10" || rel.Country != "US" || rel.Barcode != "0123456789012") {
			t.Errorf("release %+v", rel)
		}
	}
}

func TestReleaseAndRecording(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	b, ok := r.StoredData(ctx, "release", relA)
	if !ok {
		t.Fatal("release missing")
	}
	var rel struct {
		Source struct {
			Group struct {
				ID string `json:"id"`
			} `json:"release-group"`
			Media []struct {
				Format string
				Tracks []struct {
					Length    *int `json:"length"`
					Recording struct {
						ID     string `json:"id"`
						Length int    `json:"length"`
					} `json:"recording"`
					Credit []struct {
						JoinPhrase string `json:"joinphrase"`
					} `json:"artist-credit"`
				} `json:"tracks"`
			} `json:"media"`
		} `json:"source_data"`
	}
	if err := json.Unmarshal(b, &rel); err != nil {
		t.Fatal(err)
	}
	m := rel.Source.Media
	if rel.Source.Group.ID != groupGID || len(m) != 1 || m[0].Format != "CD" || len(m[0].Tracks) != 2 || m[0].Tracks[1].Length != nil ||
		m[0].Tracks[1].Recording.Length != 180000 || len(m[0].Tracks[1].Credit) != 2 || m[0].Tracks[1].Credit[0].JoinPhrase != " feat. " {
		t.Fatalf("release answer %s", b)
	}
	b, ok = r.StoredData(ctx, "recording", recGID)
	if !ok {
		t.Fatal("recording missing")
	}
	var rec struct {
		Source struct {
			First    string `json:"first-release-date"`
			Releases []any  `json:"releases"`
		} `json:"source_data"`
	}
	_ = json.Unmarshal(b, &rec)
	if rec.Source.First != "1998" || len(rec.Source.Releases) != 3 {
		t.Fatalf("recording answer %s", b)
	}
}

func TestDiscog(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	a, ok := r.Artist(ctx, artistGID)
	if !ok || len(a.Groups) != 2 || a.Count != 2 || !a.Complete {
		t.Fatalf("artist %+v", a)
	}
	al, ok := r.Album(ctx, groupGID)
	if !ok {
		t.Fatal("album missing")
	}
	// Choose: official editions with the most common track count (relA
	// has 2, relB 1+1 = 2), then the home country (GB) one: relA.
	if al.Edition == nil || al.Edition.ID != relA || al.TrackCount != 2 || al.EditionCount != 3 || !al.EditionsComplete ||
		al.Title != "First Album" || al.PrimaryType != "Album" || al.FirstReleaseDate != "1998" || len(al.Artists) != 1 {
		t.Fatalf("album %+v", al)
	}
	if al.Tracks[1].ArtistCredit != "Test Band feat. Guest" || al.Tracks[1].LengthMS != 180000 {
		t.Errorf("tracks %+v", al.Tracks)
	}
	if al.FetchedAt.Format("20060102") != "20261025" {
		t.Errorf("album fetched at %v, want the dump date", al.FetchedAt)
	}
}

func TestMissingFallsThrough(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	for _, kind := range []string{"artist", "release-group", "release", "recording"} {
		if _, ok := r.StoredData(ctx, kind, missing); ok {
			t.Errorf("%s: missing item answered", kind)
		}
		if r.Has(ctx, kind, missing) {
			t.Errorf("%s: Has missing", kind)
		}
	}
	if !r.Has(ctx, "artist", artistGID) {
		t.Error("Has artist")
	}
	if _, ok := r.Artist(ctx, missing); ok {
		t.Error("discog artist answered")
	}
	if _, ok := r.Album(ctx, missing); ok {
		t.Error("discog album answered")
	}
	if _, ok := r.StoredData(ctx, "label", artistGID); ok {
		t.Error("unknown kind answered")
	}
}

func TestNoCopyYet(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Check = time.Millisecond
	if _, ok := r.StoredData(context.Background(), "artist", artistGID); ok {
		t.Fatal("answered without a copy")
	}
	writeCopy(t, root, "20261025-001001", "First Album")
	point(t, root, "20261025-001001")
	time.Sleep(5 * time.Millisecond)
	if _, ok := r.StoredData(context.Background(), "artist", artistGID); !ok {
		t.Fatal("copy that appeared later not read")
	}
}

// TestSwapUnderLoad swaps CURRENT back and forth while readers run: every
// read answers, from one copy or the other, and old copies are closed.
func TestSwapUnderLoad(t *testing.T) {
	root := t.TempDir()
	dates := []string{"20261025-001001", "20261101-001001", "20261108-001001"}
	for i, d := range dates {
		writeCopy(t, root, d, fmt.Sprintf("Copy %d", i))
	}
	point(t, root, dates[0])
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Check = time.Microsecond // every read rereads CURRENT
	ctx := context.Background()
	var reads, fails atomic.Int64
	seen := sync.Map{}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				b, date, ok := r.Stored(ctx, "release-group", groupGID)
				reads.Add(1)
				if !ok {
					fails.Add(1)
					continue
				}
				var d struct {
					Title string `json:"title"`
				}
				if json.Unmarshal(b, &d) != nil || d.Title == "" {
					fails.Add(1)
				}
				seen.Store(date, true)
				if _, ok := r.Album(ctx, groupGID); !ok {
					fails.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Millisecond)
		point(t, root, dates[(i+1)%len(dates)])
	}
	time.Sleep(20 * time.Millisecond)
	close(stop)
	wg.Wait()
	n := 0
	seen.Range(func(_, _ any) bool { n++; return true })
	t.Logf("%d reads, %d failed, %d swaps, %d copies seen", reads.Load(), fails.Load(), r.Swaps(), n)
	if fails.Load() != 0 {
		t.Fatalf("%d of %d reads failed during swaps", fails.Load(), reads.Load())
	}
	if r.Swaps() < 10 || n != len(dates) {
		t.Fatalf("swaps %d, copies seen %d", r.Swaps(), n)
	}
}

// BenchmarkLookup reads an artist answer from the fake copy (the real
// copy is measured by cmd/mbreadbench).
func BenchmarkLookup(b *testing.B) {
	r := open(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.StoredData(ctx, "artist", artistGID); !ok {
			b.Fatal("missing")
		}
	}
}
