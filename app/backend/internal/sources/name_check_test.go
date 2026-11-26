package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func postNameCheck(t *testing.T, srvURL, body string, want int) NameCheck {
	t.Helper()
	var out NameCheck
	b := do(t, "POST", srvURL+"/api/v1/releases/name-check", body, want)
	if want == http.StatusOK {
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%v: %s", err, b)
		}
	}
	return out
}

func TestNameCheckMovie(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1")
	srv := routesServer(t, f.s, nil)
	out := postNameCheck(t, srv.URL, `{"name":"Dune.Part.Two.2024.1080p.BluRay.x264-GROUP"}`, http.StatusOK)
	if out.Title == nil || out.Title.Name != "Dune: Part Two" || out.Title.Kind != "movie" || !out.Title.Sure {
		t.Fatalf("title = %+v", out.Title)
	}
	if out.Parsed.Resolution.Value != 1080 || out.Parsed.Group.Value != "GROUP" || out.Read == nil || out.Read.Source == "" {
		t.Fatalf("read = %+v / %+v", out.Parsed, out.Read)
	}
	if len(out.Copies) != 1 || out.Copies[0].Version != "HD" || out.Copies[0].PickRules == "" || len(out.Copies[0].Reasons) == 0 {
		t.Fatalf("copies = %+v", out.Copies)
	}
	// Nothing is stored: the tool only reads.
	var n int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM releases").Scan(&n); err != nil || n != 0 {
		t.Fatalf("releases stored = %d, %v", n, err)
	}
}

func TestNameCheckRefusedCam(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1")
	srv := routesServer(t, f.s, nil)
	out := postNameCheck(t, srv.URL, `{"name":"Dune.Part.Two.2024.HDCAM.x264-NOGRP"}`, http.StatusOK)
	if len(out.Copies) != 1 || out.Copies[0].Passed || out.Copies[0].Outcome != "refused" {
		t.Fatalf("cam copies = %+v", out.Copies)
	}
}

func TestNameCheckNoTitleAndPickRules(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1")
	srv := routesServer(t, f.s, nil)
	out := postNameCheck(t, srv.URL, `{"name":"Some.Other.Film.2019.2160p.WEB-DL.DDP5.1.HDR.H.265-XYZ"}`, http.StatusOK)
	if out.Title != nil || out.Note != "no_title" || len(out.Copies) != 0 || out.Parsed.Year.Value != 2019 {
		t.Fatalf("no title = %+v", out)
	}
	var pr int64
	if err := f.s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"Some.Other.Film.2019.1080p.WEB-DL.DDP5.1.H.264-XYZ","pick_rules_id":` + strconv.FormatInt(pr, 10) + `}`
	out = postNameCheck(t, srv.URL, body, http.StatusOK)
	if len(out.Copies) != 1 || out.Copies[0].PickRules != mediafiles.GoodEverydayName || out.Copies[0].Version != "" {
		t.Fatalf("pick rules copies = %+v", out.Copies)
	}
	postNameCheck(t, srv.URL, `{"name":"x","pick_rules_id":99999}`, http.StatusNotFound)
	postNameCheck(t, srv.URL, `{"name":"x","title_id":99999}`, http.StatusNotFound)
	postNameCheck(t, srv.URL, `{"name":"   "}`, http.StatusBadRequest)
	postNameCheck(t, srv.URL, `{"name":"`+strings.Repeat("a", 1001)+`"}`, http.StatusBadRequest)
}

func TestNameCheckShowEpisode(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1", nil)
	srv := routesServer(t, f.s, nil)
	out := postNameCheck(t, srv.URL, `{"name":"Copper.Valley.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP"}`, http.StatusOK)
	if out.Title == nil || out.Title.Kind != "show" {
		t.Fatalf("title = %+v", out.Title)
	}
	if out.Parsed.Show == nil || len(out.Parsed.Show.Episodes) != 1 || out.Parsed.Show.Episodes[0] != 2 {
		t.Fatalf("show numbers = %+v", out.Parsed.Show)
	}
	if len(out.Copies) != 1 {
		t.Fatalf("copies = %+v", out.Copies)
	}
	for _, r := range out.Copies[0].Reasons {
		if r.Code == "match.episode" && r.Effect == "pass" {
			return
		}
	}
	t.Fatalf("no episode match line: %+v", out.Copies[0].Reasons)
}

func TestNameCheckOnlyThatTitle(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1", nil)
	srv := routesServer(t, f.s, nil)
	var showID int64
	if err := f.s.Read().QueryRowContext(context.Background(), "SELECT id FROM titles WHERE kind = 'show'").Scan(&showID); err != nil {
		t.Fatal(err)
	}
	out := postNameCheck(t, srv.URL, `{"name":"Dune.Part.Two.2024.1080p.BluRay.x264-GROUP","title_id":`+strconv.FormatInt(showID, 10)+`}`, http.StatusOK)
	if out.Title != nil || out.Note != "no_title" {
		t.Fatalf("matched outside the chosen title: %+v", out.Title)
	}
}
