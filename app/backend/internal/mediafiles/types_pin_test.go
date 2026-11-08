package mediafiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// catalogRecords are the library records, pinned field by field before they
// move to their owner.
func catalogRecords() map[string]any {
	return map[string]any{
		"LibraryFolder": LibraryFolder{}, "Title": Title{}, "NeverAdd": NeverAdd{}, "Part": Part{},
		"Numbers": Numbers{}, "Item": Item{}, "Version": Version{}, "Slot": Slot{}, "PickRules": PickRules{},
		"PlanStep": PlanStep{}, "PlaceResult": PlaceResult{}, "Import": Import{}, "Quality": Quality{}, "File": File{},
		"PickRulesBody": PickRulesBody{}, "OwnTrait": OwnTrait{},
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Errorf("%s changed:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestCatalogRecordLayoutIsFrozen(t *testing.T) {
	recs := catalogRecords()
	names := []string{"LibraryFolder", "Title", "NeverAdd", "Part", "Numbers", "Item", "Version", "Slot", "PickRules",
		"PlanStep", "PlaceResult", "Import", "Quality", "File", "PickRulesBody", "OwnTrait"}
	var b strings.Builder
	for _, n := range names {
		b.WriteString("== " + n + "\n" + strings.ReplaceAll(fieldLayout(recs[n]), "mediafiles.", ""))
	}
	checkGolden(t, "pin_catalog_layouts.golden.txt", b.String())
}

func TestCatalogRecordJSONIsFrozen(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	year := 1999
	recs := []any{
		Title{Meta: database.Meta{ID: 7, CreatedAt: at, UpdatedAt: at, CreatedBy: database.System, UpdatedBy: database.System}, Kind: "movie", Name: "Pin", Year: &year},
		Slot{Meta: database.Meta{ID: 3, CreatedAt: at, UpdatedAt: at}, State: SlotState("wanted")},
		File{Meta: database.Meta{ID: 4, CreatedAt: at, UpdatedAt: at}, RelativePath: "pin.mkv", Size: 42},
		GoodEveryday(),
	}
	var b strings.Builder
	for _, r := range recs {
		out, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		b.Write(out)
		b.WriteString("\n")
	}
	checkGolden(t, "pin_catalog_json.golden.json", b.String())
}

// fieldLayout lists a record's fields in order, with their tags.
func fieldLayout(v any) string {
	t := reflect.TypeOf(v)
	var b strings.Builder
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		typ := strings.NewReplacer("mediafiles.", "", "database.", "").Replace(f.Type.String())
		b.WriteString(f.Name + " " + typ + " `" + string(f.Tag) + "`\n")
	}
	return b.String()
}
