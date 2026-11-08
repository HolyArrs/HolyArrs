package api

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Shared by the calendar tests; the title-defaults tests moved to cmd/holyarrs.

type addAnswer struct {
	Title TitleView `json:"title"`
	JobID int64     `json:"job_id"`
}

func addMovie(t *testing.T, base, body string) addAnswer {
	t.Helper()
	var a addAnswer
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/titles", body, 201), &a)
	return a
}

func folder(t *testing.T, base, kind string) FolderView {
	t.Helper()
	var lf FolderView
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), kind, kind+t.Name()), 201), &lf)
	return lf
}
