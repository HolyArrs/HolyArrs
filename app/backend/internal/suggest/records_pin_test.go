package suggest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// pinFill sets every exported field to a fixed non-zero value, so a marshalled
// record shows all of its JSON names, omitempty ones included.
func pinFill(v reflect.Value, depth int) {
	if depth > 4 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		pinFill(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				pinFill(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		pinFill(s.Index(0), depth+1)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		pinFill(k, depth+1)
		e := reflect.New(v.Type().Elem()).Elem()
		pinFill(e, depth+1)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.String:
		v.SetString("s")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf("x"))
		}
	}
}

// pinGolden marshals a filled T and compares it to testdata/<name>.golden.json.
// PIN_WRITE=1 writes the file (used once, when the pin is made).
func pinGolden[T any](t *testing.T, name string) {
	t.Helper()
	var v T
	pinFill(reflect.ValueOf(&v).Elem(), 0)
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", name+".golden.json")
	if os.Getenv("PIN_WRITE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("%s JSON changed:\n got %s\nwant %s", name, got, want)
	}
}

// The Explore records pages receive keep their JSON field names (R4).
func TestPinExploreRecordJSON(t *testing.T) {
	t.Run("item", func(t *testing.T) { pinGolden[Item](t, "pin_item") })
	t.Run("row", func(t *testing.T) { pinGolden[Row](t, "pin_row") })
	t.Run("page", func(t *testing.T) { pinGolden[Page](t, "pin_page") })
	t.Run("fresh", func(t *testing.T) { pinGolden[Fresh](t, "pin_fresh") })
	t.Run("browse_section", func(t *testing.T) { pinGolden[BrowseSection](t, "pin_browse_section") })
	t.Run("category_tile", func(t *testing.T) { pinGolden[CategoryTile](t, "pin_category_tile") })
	t.Run("category_result", func(t *testing.T) { pinGolden[CategoryResult](t, "pin_category_result") })
	t.Run("genre_card", func(t *testing.T) { pinGolden[GenreCard](t, "pin_genre_card") })
	t.Run("browse_tile", func(t *testing.T) { pinGolden[BrowseTile](t, "pin_browse_tile") })
	t.Run("pool", func(t *testing.T) { pinGolden[Pool](t, "pin_pool") })
	t.Run("request", func(t *testing.T) { pinGolden[Request](t, "pin_request") })
}
