package migrate

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// An old folder with no Library folder of the same name goes to the only
// Library folder of its kind here (Sonarr's "/data/TV Shows" to the shows one).
func TestSuggestRulesByKind(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		for _, f := range [][2]string{{"/lib/movies", "movie"}, {"/lib/shows", "show"}} {
			if _, err := tx.Exec(`INSERT INTO library_folders(created_at, created_by, updated_at, updated_by, path, kind, label) VALUES ('', '{}', '', '{}', ?, ?, ?)`, f[0], f[1], f[1]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p := &Plan{
		Paths: []string{"/nowhere-xyz/Movies", "/nowhere-xyz/TV Shows", "/nowhere-xyz/Music"},
		Groups: []Group{{Items: []Item{
			{Folder: &FolderPlan{OldPath: "/nowhere-xyz/Movies", Kind: "movie"}},
			{Folder: &FolderPlan{OldPath: "/nowhere-xyz/TV Shows", Kind: "show"}},
			{Folder: &FolderPlan{OldPath: "/nowhere-xyz/Music", Kind: "music"}},
		}}},
	}
	got := map[string]string{}
	for _, r := range SuggestRules(ctx, st, p) {
		got[r.From] = r.To
	}
	if got["/nowhere-xyz/Movies"] != "/lib/movies" || got["/nowhere-xyz/TV Shows"] != "/lib/shows" {
		t.Fatalf("suggested %v", got)
	}
	if _, ok := got["/nowhere-xyz/Music"]; ok {
		t.Fatalf("music has no folder here, yet got %v", got)
	}
}
