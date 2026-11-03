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
	b := do(t, "POST", url+"/api/v1/manual-import", fmt.Sprintf(`{"items":[{"path":%q,"title_id":%d,"mode":"move"}]}`, src, sc.Files[0].Guess.TitleID), 200)
	if !strings.Contains(string(b), `"placed":1`) || !strings.Contains(string(b), `"placed_by":"move"`) {
		t.Fatalf("import %s", b)
	}
	if _, err := os.Stat(src); err == nil {
		t.Fatal("the moved file is still in the folder")
	}
}
