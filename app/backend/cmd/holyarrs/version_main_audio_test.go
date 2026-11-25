package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A Library folder's audio languages reach a movie added there, and the
// Library's bulk edit sets them on any title's main copy by "main".
// route: PUT /api/v1/titles/{id}/versions/{vid}
// route: PUT /api/v1/library-folders/{id}
func TestFolderAudioAndMainVersionEdit(t *testing.T) {
	t.Parallel()
	srv, _, _ := lbLibSetup(t)
	var lf library.FolderView
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"IT","new_title_audio":["IT"]}`, t.TempDir()), 201), &lf)
	if len(lf.NewTitleAudio) != 1 || lf.NewTitleAudio[0] != "it" {
		t.Fatalf("folder %+v", lf)
	}
	lbDo(t, "PUT", srv.URL+"/api/v1/library-folders/"+lbItoa(lf.ID), `{"new_title_audio":["??"]}`, 400)
	var added struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/titles", fmt.Sprintf(`{"id":"tmdb:693134","library_folder_id":%d}`, lf.ID), 201), &added)
	base := srv.URL + "/api/v1/titles/" + lbItoa(added.Title.ID) + "/versions/"
	b := lbDo(t, "PUT", base+"main", `{"audio_languages":["it","en"]}`, 200)
	if !strings.Contains(string(b), `"audio_languages":["it","en"]`) || !strings.Contains(string(b), `"main":true`) {
		t.Fatalf("main edit %s", b)
	}
	lbDo(t, "PUT", base+"main", `{"audio_languages":["not a code"]}`, 400)
	lbDo(t, "PUT", srv.URL+"/api/v1/titles/999/versions/main", `{"audio_languages":["it"]}`, 404)
}
