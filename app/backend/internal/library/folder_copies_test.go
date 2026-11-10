package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// copies_expected is set and cleared through the folder API, and a rename
// alone leaves it as it was.
func TestFolderCopiesExpectedAPI(t *testing.T) {
	s := open(t)
	lf, err := AddFolder(context.Background(), s, FolderInput{Path: t.TempDir(), Kind: "movie"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	h := &FolderHandlers{Store: s, DownloadPaths: func(*http.Request) []string { return nil },
		Actor: func(*http.Request) domain.Actor { return domain.System },
		Fail:  func(w http.ResponseWriter, err error) { webserver.Fail(w, http.StatusBadRequest, err.Error()) }}
	m := http.NewServeMux()
	h.Routes(m)
	put := func(body string) FolderView {
		t.Helper()
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/library-folders/"+strconv.FormatInt(lf.ID, 10), strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d %s", body, rec.Code, rec.Body)
		}
		var v FolderView
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := put(`{"copies_expected": true}`); !v.CopiesExpected {
		t.Fatal("copies_expected not set")
	}
	if v := put(`{"label": "Films"}`); !v.CopiesExpected || v.Label != "Films" {
		t.Fatalf("rename changed copies_expected: %+v", v)
	}
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/library-folders", nil))
	if !strings.Contains(rec.Body.String(), `"copies_expected":true`) {
		t.Fatalf("list does not show copies_expected: %s", rec.Body)
	}
	if v := put(`{"copies_expected": false}`); v.CopiesExpected {
		t.Fatal("copies_expected not cleared")
	}
}
