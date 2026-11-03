package api

import (
	"encoding/json"
	"fmt"
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
	srv, _, _ := libSetup(t)
	url := srv.URL
	lib := t.TempDir()
	do(t, "POST", url+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201)
	do(t, "POST", url+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	loose := t.TempDir()
	src := filepath.Join(loose, "Dune.Part.Two.2024.1080p.WEB-DL.x264-HOLYARRS.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", url+"/api/v1/manual-import/scan", `{"path":"relative"}`, 400)
	do(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose+"/x/../.."), 400)
	// Outside the Library folders, the Downloaders' folders and the extra
	// import folders nothing is read; the answer names the places.
	b := do(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose), 400)
	var refused struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(b, &refused)
	if !strings.Contains(refused.Error, "extra import folders") || !strings.Contains(refused.Error, lib) {
		t.Fatalf("outside %s", b)
	}
	do(t, "PUT", url+"/api/v1/settings/import-folders", `{"extra_folders":["relative"]}`, 400)
	do(t, "PUT", url+"/api/v1/settings/import-folders", `{"extra_folders":["/data/../etc"]}`, 400)
	b = do(t, "PUT", url+"/api/v1/settings/import-folders", fmt.Sprintf(`{"extra_folders":[%q]}`, loose+string(filepath.Separator)), 200)
	var places struct {
		Library []string `json:"library_folders"`
		Extra   []string `json:"extra_folders"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/settings/import-folders", "", 200), &places)
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
	_ = json.Unmarshal(do(t, "POST", url+"/api/v1/manual-import/scan", fmt.Sprintf(`{"path":%q}`, loose), 200), &sc)
	if len(sc.Files) != 1 || sc.Files[0].Guess.TitleID == 0 || sc.Files[0].Guess.SlotID == 0 {
		t.Fatalf("scan %+v", sc)
	}
	do(t, "POST", url+"/api/v1/manual-import", `{"items":[]}`, 400)
	b = do(t, "POST", url+"/api/v1/manual-import", fmt.Sprintf(`{"items":[{"path":%q,"title_id":%d,"mode":"move"}]}`, src, sc.Files[0].Guess.TitleID), 200)
	if !strings.Contains(string(b), `"placed":1`) || !strings.Contains(string(b), `"placed_by":"move"`) {
		t.Fatalf("import %s", b)
	}
	if _, err := os.Stat(src); err == nil {
		t.Fatal("the moved file is still in the folder")
	}
}
