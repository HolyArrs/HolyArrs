package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// route: GET /api/v1/holding
// route: POST /api/v1/holding/restore
func TestHoldingRestoreAPI(t *testing.T) {
	t.Parallel()
	url, _, lib, id, fid := organiseWorld(t)
	do(t, "POST", url+"/api/v1/titles/"+itoa(id)+"/delete", `{"delete_files":true,"apply":true}`, 200)
	var list struct {
		Files []struct {
			ID           int64  `json:"id"`
			Path         string `json:"path"`
			TitleID      int64  `json:"title_id"`
			TitleDeleted bool   `json:"title_deleted"`
			ExpiresAt    string `json:"expires_at"`
		} `json:"files"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/holding", "", 200), &list)
	if len(list.Files) != 1 || list.Files[0].ID != fid || list.Files[0].TitleID != id || !list.Files[0].TitleDeleted || list.Files[0].ExpiresAt == "" {
		t.Fatalf("list %+v", list)
	}
	do(t, "POST", url+"/api/v1/holding/restore", `{"ids":[]}`, 400)
	b := string(do(t, "POST", url+"/api/v1/holding/restore", `{"ids":[`+itoa(fid)+`]}`, 200))
	if !strings.Contains(b, `"result":"restored"`) || !strings.Contains(b, `"titles_back":[`+itoa(id)+`]`) {
		t.Fatalf("restore %s", b)
	}
	if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(list.Files[0].Path))); err != nil {
		t.Fatal("not back on disk")
	}
	if b := string(do(t, "GET", url+"/api/v1/titles/"+itoa(id), "", 200)); !strings.Contains(b, `"state":"in_library"`) {
		t.Fatalf("title %s", b)
	}
	if b := string(do(t, "GET", url+"/api/v1/holding", "", 200)); b != `{"files":[]}`+"\n" && !strings.Contains(b, `"files":[]`) {
		t.Fatalf("after %s", b)
	}
}
