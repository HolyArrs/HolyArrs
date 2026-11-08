package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// AddOutsideList reads library_folders and pick_rules (other modules' tables) while it
// saves: with no folder given it takes the lowest id of the list's kind, and
// a missing partner row (no Pick rules chosen) is stored as NULL.
func TestPinAddReadsFoldersAndPickRules(t *testing.T) {
	e := setup(t) // one movie folder
	ctx := context.Background()
	// A crowd of other folders and Pick rules around the one that counts.
	for i := 0; i < 40; i++ {
		kind := "show"
		if i%2 == 0 {
			kind = "movie"
		}
		if _, err := library.AddFolder(ctx, e.s, library.FolderInput{Path: t.TempDir(), Kind: kind, Label: fmt.Sprintf("F%d", i)}, nil, domain.System); err != nil {
			t.Fatalf("folder %d: %v", i, err)
		}
	}
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "A", Kind: ListTMDB, Ref: "42"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if l.LibraryFolderID != e.lf.ID || l.PickRulesID != nil || l.TitleKind != "movie" || l.SyncEveryHours != 12 || !l.Enabled || !l.KeepUpToDate {
		t.Fatalf("defaults %+v", l)
	}
	// A show list takes the first show folder, not the movie one.
	s, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "B", Kind: ListTMDB, Ref: "43", TitleKind: "show"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var first int64
	_ = e.s.Read().QueryRow("SELECT min(id) FROM library_folders WHERE kind = 'show'").Scan(&first)
	if s.LibraryFolderID != first || s.Seasons != library.SeasonsAll || s.NewSeasons != "all" || s.ReleaseGate != "" {
		t.Fatalf("show list %+v (first show folder %d)", s, first)
	}
	// Pick rules must list the list's kind.
	var showOnly int64
	err = e.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO pick_rules (created_at, created_by, updated_at, updated_by, name, kinds, body) VALUES ('2026-10-13T12:00:00Z', '{}', '2026-10-13T12:00:00Z', '{}', 'ShowsOnly', '["show"]', '{}')`)
		if err != nil {
			return err
		}
		showOnly, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "C", Kind: ListTMDB, Ref: "44", PickRulesID: showOnly}, domain.System); !errors.Is(err, ErrRulesKind) {
		t.Fatalf("movie list on show rules: %v", err)
	}
	ok, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "D", Kind: ListTMDB, Ref: "45", TitleKind: "show", PickRulesID: showOnly}, domain.System)
	if err != nil || ok.PickRulesID == nil || *ok.PickRulesID != showOnly {
		t.Fatalf("show list on show rules: %+v %v", ok, err)
	}
	// A show folder given for a movie list is refused.
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "E", Kind: ListTMDB, Ref: "46", LibraryFolderID: first}, domain.System); !errors.Is(err, library.ErrNotMovieFolder) {
		t.Fatalf("wrong folder kind: %v", err)
	}
}

// AllOutsideLists lists by name ignoring case; GetOutsideList and RemoveOutsideList report a missing id the same way.
func TestPinListOrderAndMissing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for i, n := range []string{"bravo", "Alpha", "charlie", "ALPHA2", "delta"} {
		if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: n, Kind: ListTMDB, Ref: fmt.Sprint(100 + i)}, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	all, err := AllOutsideLists(ctx, e.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range all {
		names = append(names, l.Name)
	}
	if fmt.Sprint(names) != "[Alpha ALPHA2 bravo charlie delta]" {
		t.Fatalf("order %v", names)
	}
	if err := RemoveOutsideList(ctx, e.s, 9999); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("remove missing: %v", err)
	}
	if _, err := GetOutsideList(ctx, e.s.Read(), 9999); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	empty := setup(t)
	if err := RemoveOutsideList(ctx, empty.s, 1); err == nil {
		t.Fatal("remove from empty")
	}
	if got, _ := AllOutsideLists(ctx, empty.s.Read()); got == nil || len(got) != 0 {
		t.Fatalf("empty list must be [] not nil: %#v", got)
	}
}

// Record JSON names are frozen (R4).
func TestPinListRecordJSON(t *testing.T) {
	for name, v := range map[string]any{"outside_lists_input": &OutsideListInput{}, "outside_lists_result": &OutsideListResult{}} {
		pinFillList(reflect.ValueOf(v).Elem())
		got, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", name+".golden.json")
		if os.Getenv("PIN_WRITE") == "1" {
			_ = os.MkdirAll("testdata", 0o755)
			_ = os.WriteFile(path, append(got, '\n'), 0o644)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got)+"\n" {
			t.Fatalf("%s JSON changed:\n got %s\nwant %s", name, got, want)
		}
	}
}

func pinFillList(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		pinFillList(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			pinFillList(v.Field(i))
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		pinFillList(s.Index(0))
		v.Set(s)
	case reflect.String:
		v.SetString("s")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	}
}
