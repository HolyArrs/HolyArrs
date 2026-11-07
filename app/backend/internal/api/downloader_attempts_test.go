package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestDownloaderAttemptErrorsArePublicSentinels(t *testing.T) {
	for _, sentinel := range []error{downloader.ErrUncertain, downloader.ErrAttemptStopped, downloader.ErrPendingAttempt} {
		w := httptest.NewRecorder()
		downloaderError(w, errors.Join(sentinel, errors.New("private provider and database detail")))
		if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "private provider") || !strings.Contains(w.Body.String(), sentinel.Error()) {
			t.Fatal("attempt conflict leaked joined cause", w.Code)
		}
	}
}

func TestDownloaderAttemptRoutePolicy(t *testing.T) {
	patterns := []string{
		"GET /api/v1/downloaders/{id}/attempts",
		"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon",
	}
	withSettings := auth.Who{Role: auth.RoleAdmin, Perms: []auth.Perm{auth.PermSettings}}
	withoutSettings := auth.Who{Role: auth.RoleAdmin}
	member := auth.Who{Role: auth.RoleMember, Perms: []auth.Perm{auth.PermAsk}}
	for _, pattern := range patterns {
		if got := auth.RoutePerms[pattern]; got != auth.PermSettings {
			t.Errorf("%s permission = %q, want settings", pattern, got)
		}
		if got := auth.RouteScope(pattern); got != auth.ScopeAdmin {
			t.Errorf("%s scope = %q, want admin", pattern, got)
		}
		if !withSettings.Has(auth.RoutePerms[pattern]) {
			t.Errorf("settings admin cannot use %s", pattern)
		}
		if withoutSettings.Has(auth.RoutePerms[pattern]) {
			t.Errorf("admin without settings can use %s", pattern)
		}
		if member.Has(auth.RoutePerms[pattern]) {
			t.Errorf("member can use %s", pattern)
		}
	}
}

func TestDownloaderAttemptAbandonNeedsCSRF(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	vault, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := auth.New(s, auth.Options{Vault: vault})
	srv := httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: vault, Live: webserver.NewLiveHub(webserver.LiveOptions{}),
		Backups: &backup.Manager{Store: s, Dir: dir, Version: "test"}}).Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(csrf string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/downloaders/1/attempts/1/abandon", nil)
		if csrf != "" {
			req.Header.Set(auth.CSRFHeader, csrf)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	setupBody := `{"name":"luis","password":"a long test passphrase"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/setup", strings.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var session struct {
		CSRF string `json:"csrf"`
	}
	if resp.StatusCode != http.StatusCreated || json.NewDecoder(resp.Body).Decode(&session) != nil || session.CSRF == "" {
		t.Fatalf("owner setup status %d", resp.StatusCode)
	}
	if status, _ := call(""); status != http.StatusForbidden {
		t.Fatalf("abandon without CSRF = %d, want 403", status)
	}
	if status, body := call(session.CSRF); status != http.StatusNotFound || !strings.Contains(body, "no such send attempt") {
		t.Fatalf("abandon with CSRF = %d %s, want route-level 404", status, body)
	}
}

// route: GET /api/v1/downloaders/{id}/attempts
// route: POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon
func TestDownloaderAttemptRoutesRedactAndAbandon(t *testing.T) {
	srv, s := dlServer(t)
	var downloaderID, otherDownloaderID, attemptID int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var pickRulesID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pickRulesID); err != nil {
			return err
		}
		folder := &domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Attempt movies", Health: "ok", SameDiskAs: []int64{}}
		title := &domain.Title{Kind: "movie", GateDriver: "none", Name: "Signal Harbor", SortName: "signal harbor", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		for _, record := range []struct {
			table string
			value any
		}{{"library_folders", folder}, {"titles", title}} {
			if _, err := database.Insert(tx, record.table, record.value, domain.System); err != nil {
				return err
			}
		}
		item := &domain.Item{TitleID: title.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", item, domain.System); err != nil {
			return err
		}
		version := &domain.Version{TitleID: title.ID, Label: "HD", PickRulesID: pickRulesID, LibraryFolderID: folder.ID, FolderName: "Signal Harbor", IsMain: true}
		if _, err := database.Insert(tx, "versions", version, domain.System); err != nil {
			return err
		}
		slot := &domain.Slot{ItemID: item.ID, VersionID: version.ID, State: domain.SlotChosen}
		if _, err := database.Insert(tx, "slots", slot, domain.System); err != nil {
			return err
		}
		source := &domain.Source{Name: "Attempt source", Protocol: "usenet", Format: "newznab", URL: "http://example.invalid", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", source, domain.System); err != nil {
			return err
		}
		release := &domain.Release{SourceID: source.ID, Name: "Signal.Harbor.2026.1080p-WEB-HOLYARRS", LinkRef: "http://example.invalid/release.nzb", Protocol: "usenet", Size: 1,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{slot.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", release, domain.System); err != nil {
			return err
		}
		downloader := &domain.Downloader{Name: "Attempt NZBGet", Adapter: "nzbget", Protocol: "usenet", URL: "http://example.invalid", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		other := &domain.Downloader{Name: "Other NZBGet", Adapter: "nzbget", Protocol: "usenet", URL: "http://other.invalid", Category: "holyarrs", Priority: 2,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		for _, value := range []*domain.Downloader{downloader, other} {
			if _, err := database.Insert(tx, "downloaders", value, domain.System); err != nil {
				return err
			}
		}
		downloaderID, otherDownloaderID = downloader.ID, other.ID
		result, err := tx.Exec(`INSERT INTO downloader_attempts(downloader_id, release_id, slot_id, marker, category, state, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, 'pending', ?, ?)`, downloader.ID, release.ID, slot.ID, "opaque-attempt-marker", "private-category", "2026-10-15T20:00:00Z", "2026-10-15T20:00:00Z")
		if err != nil {
			return err
		}
		attemptID, err = result.LastInsertId()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	body := do(t, "GET", srv.URL+"/api/v1/downloaders/"+itoa(downloaderID)+"/attempts", "", 200)
	if strings.Contains(string(body), "opaque-attempt-marker") || strings.Contains(string(body), "private-category") || strings.Contains(string(body), "example.invalid") {
		t.Fatalf("private attempt detail escaped: %s", body)
	}
	var answer struct {
		Attempts []map[string]any `json:"attempts"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Attempts) != 1 {
		t.Fatalf("attempts %#v", answer.Attempts)
	}
	allowed := map[string]bool{"id": true, "downloader_id": true, "release_id": true, "slot_id": true, "state": true, "created_at": true, "updated_at": true}
	for key := range answer.Attempts[0] {
		if !allowed[key] {
			t.Fatalf("unexpected public field %q", key)
		}
	}
	if len(answer.Attempts[0]) != len(allowed) || answer.Attempts[0]["state"] != "pending" {
		t.Fatalf("public attempt %#v", answer.Attempts[0])
	}

	do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(otherDownloaderID)+"/attempts/"+itoa(attemptID)+"/abandon", "", 404)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(downloaderID)+"/attempts/"+itoa(attemptID)+"/abandon", "", 204)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(downloaderID)+"/attempts/"+itoa(attemptID)+"/abandon", "", 204)
	var actorText string
	if err := s.Read().QueryRow("SELECT abandoned_by FROM downloader_attempts WHERE id = ?", attemptID).Scan(&actorText); err != nil {
		t.Fatal(err)
	}
	var abandonedBy domain.Actor
	if err := json.Unmarshal([]byte(actorText), &abandonedBy); err != nil || abandonedBy != (domain.Actor{Kind: "user", ID: 1}) {
		t.Fatal("abandon did not retain the API actor", abandonedBy, err)
	}
	body = do(t, "GET", srv.URL+"/api/v1/downloaders/"+itoa(downloaderID)+"/attempts", "", 200)
	if !strings.Contains(string(body), `"state":"abandoned"`) {
		t.Fatalf("not abandoned: %s", body)
	}
	do(t, "GET", srv.URL+"/api/v1/downloaders/99999/attempts", "", 404)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(downloaderID)+"/attempts/not-a-number/abandon", "", 404)
}
