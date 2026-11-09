package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// route: POST /api/v1/manual-import/scan
// route: POST /api/v1/manual-import
// route: GET /api/v1/settings/import-folders
// route: PUT /api/v1/settings/import-folders
func TestImportByHandAPI(t *testing.T) {
	t.Parallel()
	srv, _, _ := lbLibSetup(t)
	url := srv.URL
	lib := t.TempDir()
	lbDo(t, "POST", url+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201)
	lbDo(t, "POST", url+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	loose := t.TempDir()
	src := filepath.Join(loose, "Dune.Part.Two.2024.1080p.WEB-DL.x264-HOLYARRS.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	lbDo(t, "POST", url+"/api/v1/manual-import/scan", `{"path":"relative"}`, 400)
	lbDo(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose+"/x/../.."), 400)
	// Outside the Library folders, the Downloaders' folders and the extra
	// import folders nothing is read; the answer names the places.
	b := lbDo(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose), 400)
	var refused struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(b, &refused)
	if !strings.Contains(refused.Error, "extra import folders") || !strings.Contains(refused.Error, lib) {
		t.Fatalf("outside %s", b)
	}
	lbDo(t, "PUT", url+"/api/v1/settings/import-folders", `{"extra_folders":["relative"]}`, 400)
	lbDo(t, "PUT", url+"/api/v1/settings/import-folders", `{"extra_folders":["/data/../etc"]}`, 400)
	b = lbDo(t, "PUT", url+"/api/v1/settings/import-folders", fmt.Sprintf(`{"extra_folders":[%q]}`, loose+string(filepath.Separator)), 200)
	var places struct {
		Library []string `json:"library_folders"`
		Extra   []string `json:"extra_folders"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", url+"/api/v1/settings/import-folders", "", 200), &places)
	if len(places.Extra) != 1 || places.Extra[0] != loose || len(places.Library) != 1 || places.Library[0] != lib {
		t.Fatalf("places %s / %+v", b, places)
	}
	var sc struct {
		Files []struct {
			Path  string `json:"path"`
			Guess struct {
				TitleID int64 `json:"title_id"`
				SlotID  int64 `json:"slot_id"`
			} `json:"guess"`
		} `json:"files"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose), 200), &sc)
	if len(sc.Files) != 1 || sc.Files[0].Guess.TitleID == 0 || sc.Files[0].Guess.SlotID == 0 {
		t.Fatalf("scan %+v", sc)
	}
	lbDo(t, "POST", url+"/api/v1/manual-import", `{"items":[]}`, 400)
	b = lbDo(t, "POST", url+"/api/v1/manual-import", fmt.Sprintf(`{"items":[{"path":%q,"title_id":%d,"mode":"move"}]}`, src, sc.Files[0].Guess.TitleID), 200)
	if !strings.Contains(string(b), `"placed":1`) || !strings.Contains(string(b), `"placed_by":"move"`) {
		t.Fatalf("import %s", b)
	}
	if _, err := os.Stat(src); err == nil {
		t.Fatal("the moved file is still in the folder")
	}
}

// The extra import folders widen what import by hand may read, so only the
// owner or an admin signed in with a password may change them: not a member
// with the settings permission, and not a request let in without signing in.
func TestImportFoldersNeedOwnerOrAdmin(t *testing.T) {
	t.Parallel()
	r := lbNewAskRig(t)
	body := fmt.Sprintf(`{"extra_folders":[%q]}`, filepath.Join(t.TempDir(), "imports"))
	r.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", r.ana.id), `{"perms":["ask","settings"]}`, 200)
	r.ana.call("GET", "/api/v1/settings/import-folders", "", 200)
	r.ana.call("PUT", "/api/v1/settings/import-folders", body, 403)
	r.owner.call("PUT", "/api/v1/settings/import-folders", body, 200)
	r.owner.call("PUT", "/api/v1/users/"+lbItoa(r.bo.id), `{"role":"admin"}`, 200)
	r.bo.call("PUT", "/api/v1/settings/import-folders", body, 200)
	r.owner.call("PUT", "/api/v1/settings/import-folders", `{"extra_folders":["/"]}`, 400)

	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8"]}`, 200)
	req, _ := http.NewRequest("PUT", r.srv.URL+"/api/v1/settings/import-folders", strings.NewReader(`{"extra_folders":["/etc"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("local skip: %d %s", resp.StatusCode, b)
	}
}
