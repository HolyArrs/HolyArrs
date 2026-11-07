package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

func addSummaryTitle(t *testing.T, s *database.Store, kind, name string, slotState domain.SlotState) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title := domain.Title{
			Kind: kind, Name: name, SortName: strings.ToLower(name), MetadataSource: "local",
			WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all",
			ReleaseGate: "released", GateDriver: "none", Status: "released",
		}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		if kind != "movie" || slotState == "" {
			return nil
		}
		var pickRulesID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pickRulesID); err != nil {
			return err
		}
		folder := domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: name, Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", &folder, domain.System); err != nil {
			return err
		}
		item := domain.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
		if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
			return err
		}
		version := domain.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: folder.ID, PickRulesID: pickRulesID}
		if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "slots", &domain.Slot{ItemID: item.ID, VersionID: version.ID, State: slotState}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readLibrarySummary(t *testing.T, base string) struct {
	Total   int                  `json:"total"`
	Missing []libraryMissingView `json:"missing"`
} {
	t.Helper()
	var got struct {
		Total   int                  `json:"total"`
		Missing []libraryMissingView `json:"missing"`
	}
	decodeJSON(t, do(t, http.MethodGet, base+"/api/v1/library-summary", "", http.StatusOK), &got)
	return got
}

func TestLibrarySummaryHidesTitlesOwnedByOffParts(t *testing.T) {
	f := newCtl(t, false)
	addSummaryTitle(t, f.s, "movie", "Missing Movie", domain.SlotWanted)
	addSummaryTitle(t, f.s, "show", "Visible Show", "")
	addSummaryTitle(t, f.s, "artist", "Visible Artist", "")

	got := readLibrarySummary(t, f.srv.URL)
	if got.Total != 3 || len(got.Missing) != 1 || got.Missing[0].Name != "Missing Movie" {
		t.Fatalf("all parts on: %+v", got)
	}

	confirmTwice(t, f.srv.URL+"/api/v1/parts/music/off", http.StatusOK)
	got = readLibrarySummary(t, f.srv.URL)
	if got.Total != 2 || len(got.Missing) != 1 {
		t.Fatalf("music off: %+v", got)
	}

	confirmTwice(t, f.srv.URL+"/api/v1/parts/movies/off", http.StatusOK)
	got = readLibrarySummary(t, f.srv.URL)
	if got.Total != 1 || len(got.Missing) != 0 {
		t.Fatalf("music and movies off: %+v", got)
	}
}

func TestLibrarySummaryKeepsAllKindsVisibleInSafeMode(t *testing.T) {
	f := newCtlMode(t, false, true)
	addSummaryTitle(t, f.s, "movie", "Safe Movie", domain.SlotSearching)
	addSummaryTitle(t, f.s, "show", "Safe Show", "")
	addSummaryTitle(t, f.s, "artist", "Safe Artist", "")

	got := readLibrarySummary(t, f.srv.URL)
	if got.Total != 3 || len(got.Missing) != 1 || got.Missing[0].Name != "Safe Movie" {
		t.Fatalf("safe mode summary: %+v", got)
	}
}

func TestLibrarySummaryCompletesForFullSizeLibrary(t *testing.T) {
	f := newCtl(t, false)
	var pickRulesID int64
	folder := domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Scale movies", Health: "ok", SameDiskAs: []int64{}}
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pickRulesID); err != nil {
			return err
		}
		_, err := database.Insert(tx, "library_folders", &folder, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	const titleRows = `WITH RECURSIVE n(x) AS (SELECT ? UNION ALL SELECT x + 1 FROM n WHERE x < ?)
			INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added, status, state)
			SELECT x, '2026-10-15T00:00:00Z', '{"kind":"system"}', '2026-10-15T00:00:00Z', '{"kind":"system"}',
				CASE WHEN x <= 5000 THEN 'movie' WHEN x <= 7500 THEN 'show' ELSE 'artist' END,
				printf('Title %05d', x), printf('title %05d', x), 'local', 'by_hand', 'released', 'active' FROM n`
	const versionRows = `WITH RECURSIVE n(x) AS (SELECT ? UNION ALL SELECT x + 1 FROM n WHERE x < ?)
			INSERT INTO versions (id, created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
			SELECT x, '2026-10-15T00:00:00Z', '{"kind":"system"}', '2026-10-15T00:00:00Z', '{"kind":"system"}',
				x, 'Main', ?, ?, printf('Title %05d', x), 1 FROM n`
	const itemRows = `WITH RECURSIVE n(x) AS (SELECT ? UNION ALL SELECT x + 1 FROM n WHERE x < ?)
			INSERT INTO items (id, created_at, created_by, updated_at, updated_by, title_id, kind, name)
			SELECT x, '2026-10-15T00:00:00Z', '{"kind":"system"}', '2026-10-15T00:00:00Z', '{"kind":"system"}',
				((x - 1) % 5000) + 1, 'movie', printf('Item %05d', x) FROM n`
	const slotRows = `WITH RECURSIVE n(x) AS (SELECT ? UNION ALL SELECT x + 1 FROM n WHERE x < ?)
			INSERT INTO slots (id, created_at, created_by, updated_at, updated_by, item_id, version_id, state)
			SELECT x, '2026-10-15T00:00:00Z', '{"kind":"system"}', '2026-10-15T00:00:00Z', '{"kind":"system"}',
				x, ((x - 1) % 5000) + 1, CASE WHEN x <= 5000 THEN 'wanted' ELSE 'in_library' END FROM n`
	// Keep fixture writes below the normal transaction cap under race
	// instrumentation. The measured read retains its separate five-second cap.
	for _, seed := range []struct {
		query string
		rows  int
		args  []any
	}{
		{titleRows, 10000, nil},
		{versionRows, 5000, []any{pickRulesID, folder.ID}},
		{itemRows, 55000, nil},
		{slotRows, 55000, nil},
	} {
		for first := 1; first <= seed.rows; first += 1000 {
			args := append([]any{first, min(first+999, seed.rows)}, seed.args...)
			if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec(seed.query, args...)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	started := time.Now()
	resp, err := client.Get(f.srv.URL + "/api/v1/library-summary")
	if err != nil {
		t.Fatalf("full-size summary did not finish within 5s: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("full-size summary: %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Total   int                  `json:"total"`
		Missing []libraryMissingView `json:"missing"`
	}
	decodeJSON(t, body, &got)
	if got.Total != 10000 || len(got.Missing) != 8 {
		t.Fatalf("full-size summary after %s: %+v", time.Since(started), got)
	}
}

func TestLibrarySummaryAuthAndReadScope(t *testing.T) {
	r := newAskRig(t)
	addSummaryTitle(t, r.st, "show", "Member Show", "")
	do(t, http.MethodGet, r.srv.URL+"/api/v1/library-summary", "", http.StatusUnauthorized)
	var memberSummary struct {
		Total int `json:"total"`
	}
	decodeJSON(t, r.ana.call(http.MethodGet, "/api/v1/library-summary", "", http.StatusOK), &memberSummary)
	if memberSummary.Total != 1 {
		t.Fatalf("member summary: %+v", memberSummary)
	}

	makeKey := func(label, scope string) string {
		t.Helper()
		var got struct {
			Key string `json:"key"`
		}
		body := r.owner.call(http.MethodPost, "/api/v1/auth/keys", `{"label":"`+label+`","scopes":["`+scope+`"]}`, http.StatusCreated)
		if err := json.Unmarshal(body, &got); err != nil || got.Key == "" {
			t.Fatalf("create %s key: %s: %v", scope, body, err)
		}
		return got.Key
	}

	if status, body := keyCall(t, r.srv.URL, makeKey("summary read", "read"), http.MethodGet, "/api/v1/library-summary", ""); status != http.StatusOK {
		t.Fatalf("read key: %d %s", status, body)
	} else {
		var readSummary struct {
			Total int `json:"total"`
		}
		decodeJSON(t, []byte(body), &readSummary)
		if readSummary.Total != 1 {
			t.Fatalf("read summary: %+v", readSummary)
		}
	}
	if status, body := keyCall(t, r.srv.URL, makeKey("summary write", "write"), http.MethodGet, "/api/v1/library-summary", ""); status != http.StatusForbidden {
		t.Fatalf("write key: %d %s", status, body)
	}
}
