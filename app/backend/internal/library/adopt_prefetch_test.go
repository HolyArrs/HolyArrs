package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// movieServer is a metadata server stand-in: each search answers the movie
// its words name ("Film 7" is TMDB 7, 2001). batch says whether it has the
// batch route.
func movieServer(t *testing.T, batch bool) (string, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var batches, singles atomic.Int64
	answer := func(read string) json.RawMessage {
		u, _ := url.Parse("/" + strings.TrimPrefix(read, "/"))
		var n int
		fmt.Sscanf(u.Query().Get("query"), "Film %d", &n)
		return json.RawMessage(fmt.Sprintf(`{"results":[{"id":%d,"title":"Film %d","release_date":"2001-01-01"}]}`, n, n))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == metadata.BatchTMDBPath && batch:
			batches.Add(1)
			var items []map[string]any
			for _, read := range r.URL.Query()["r"] {
				items = append(items, map[string]any{"status": 200, "body": answer(read)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case strings.HasPrefix(r.URL.Path, metadata.TMDBPath+"/"):
			singles.Add(1)
			_, _ = w.Write(answer(strings.TrimPrefix(r.URL.Path, metadata.TMDBPath) + "?" + r.URL.RawQuery))
		default:
			w.WriteHeader(403)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &batches, &singles
}

func scanFolders(t *testing.T, batch bool, n int) (AdoptPlan, int64, int64) {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	for i := 1; i <= n; i++ {
		dir := filepath.Join(lf.Path, fmt.Sprintf("Film %d (2001)", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f.mkv"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base, batches, singles := movieServer(t, batch)
	md := &metadata.TMDB{BaseURL: base + metadata.TMDBPath, Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
	plan, err := PlanAdopt(context.Background(), s.Read(), md, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	return plan, batches.Load(), singles.Load()
}

func TestScanLooksFoldersUpInBatches(t *testing.T) {
	plan, batches, singles := scanFolders(t, true, 100)
	if plan.Counts["matched"] != 100 {
		t.Fatalf("counts %v", plan.Counts)
	}
	if batches != 3 || singles != 0 {
		t.Fatalf("batches %d singles %d", batches, singles)
	}
	for _, f := range plan.Folders {
		if f.Match == nil || "Film "+f.Match.TMDB+" (2001)" != f.Folder {
			t.Fatalf("folder %s matched %+v", f.Folder, f.Match)
		}
	}
}

func TestScanOnTodaysServerLooksFoldersUpOneByOne(t *testing.T) {
	plan, batches, singles := scanFolders(t, false, 30)
	if plan.Counts["matched"] != 30 || batches != 0 || singles != 30 {
		t.Fatalf("counts %v batches %d singles %d", plan.Counts, batches, singles)
	}
}

func TestScanNestedDuplicateNamesAcrossBatches(t *testing.T) {
	for _, batch := range []bool{true, false} {
		t.Run(fmt.Sprintf("batch=%v", batch), func(t *testing.T) {
			s := open(t)
			lf := addFolder(t, s)
			// The same basename occurs more than two windows apart. A basename
			// index makes the first folder wait for a batch the producer cannot
			// reach until this scan advances.
			folders := []string{"A/Film 1 (2001)", "Z/Film 1 (2001)"}
			for i := 2; i <= 3*scanWindow+1; i++ {
				folders = append(folders, fmt.Sprintf("M/Film %d (2001)", i))
			}
			for _, folder := range folders {
				dir := filepath.Join(lf.Path, filepath.FromSlash(folder))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "f.mkv"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			base, _, _ := movieServer(t, batch)
			md := &metadata.TMDB{BaseURL: base + metadata.TMDBPath, Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			plan, err := PlanAdopt(ctx, s.Read(), md, lf.ID)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Counts["matched"] != len(folders) || len(plan.Folders) != len(folders) {
				t.Fatalf("matched %d of %d folders: %+v", plan.Counts["matched"], len(folders), plan.Counts)
			}
			seen := map[string]bool{}
			for _, f := range plan.Folders {
				seen[f.Folder] = true
				if f.Match == nil || "Film "+f.Match.TMDB+" (2001)" != filepath.Base(f.Folder) {
					t.Fatalf("folder %s matched %+v", f.Folder, f.Match)
				}
			}
			for _, folder := range folders {
				if !seen[folder] {
					t.Errorf("missing folder %s", folder)
				}
			}
		})
	}
}
