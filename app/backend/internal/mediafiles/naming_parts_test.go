package mediafiles

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Settings saved before each part had its own rename, colon and
// character rules give every part the shared ones; after that each part
// keeps its own.
func TestNamingPartsUpgradeAndKeepApart(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for k, v := range map[string]string{"colon": ColonRemove, "illegal": IllegalRemove, "rename_on_import": "false"} {
			if err := database.PutSetting(tx, group, k, v, database.System, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ns, err := ReadNaming(ctx, s.Read())
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{PartMovies, PartShows, PartMusic} {
		if p := ns.For(part); p.Colon != ColonRemove || p.Illegal != IllegalRemove || p.RenameOnImport {
			t.Fatalf("%s after upgrade: %+v", part, p)
		}
	}
	ns.ShowColon, ns.ShowRenameOnImport, ns.MusicIllegal = ColonDash, true, IllegalReplace
	if err := PutNamingSettings(ctx, s, ns, database.System); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadNaming(ctx, s.Read())
	if got != ns {
		t.Fatalf("saved %+v, read %+v", ns, got)
	}
	if got.Colon != ColonRemove || got.RenameOnImport || got.MusicColon != ColonRemove || got.MusicRenameOnImport {
		t.Fatalf("one part changed another: %+v", got)
	}
	// Each part names with its own colon rule.
	f := NameFacts{Title: "Dune: Part Two", Year: 2024, Ext: ".mkv"}
	if m, sh := Folder(got, f), ShowFolder(got, f); m != "Dune Part Two (2024)" || sh != "Dune- Part Two (2024)" {
		t.Fatalf("movie %q show %q", m, sh)
	}
	ns.ShowColon = "nope"
	if err := ns.Validate(); err == nil {
		t.Fatal("a bad show colon passed")
	}
}

// An undo record kept before each part had its own rules gives every part the shared rules.
func TestNamingFromOldJSON(t *testing.T) {
	ns, err := NamingFromJSON([]byte(`{"file_pattern":"{title}","folder_pattern":"{title}","colon":"dash","illegal":"remove","rename_on_import":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if ns.ShowColon != ColonDash || ns.MusicIllegal != IllegalRemove || ns.ShowRenameOnImport || ns.MusicRenameOnImport {
		t.Fatalf("%+v", ns)
	}
	ns, _ = NamingFromJSON([]byte(`{"colon":"dash","illegal":"remove","rename_on_import":false,"show_colon":"smart","show_illegal":"replace","show_rename_on_import":true,"music_colon":"remove","music_illegal":"replace","music_rename_on_import":true}`))
	if ns.ShowColon != ColonSmart || !ns.ShowRenameOnImport || ns.MusicColon != ColonRemove {
		t.Fatalf("%+v", ns)
	}
}
