package requests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// pinFill sets every field of v to a fixed non-zero value, so a marshalled
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

// pinGolden marshals a filled value of T and compares it to testdata/<name>.golden.json.
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

// The records the requests module returns keep their JSON field names (R4).
func TestPinRecordJSON(t *testing.T) {
	t.Run("view", func(t *testing.T) { pinGolden[View](t, "view") })
	t.Run("report_view", func(t *testing.T) { pinGolden[ReportView](t, "report_view") })
	t.Run("followed", func(t *testing.T) { pinGolden[Followed](t, "followed") })
	t.Run("settings", func(t *testing.T) { pinGolden[Settings](t, "settings") })
	t.Run("person_limit", func(t *testing.T) { pinGolden[PersonLimit](t, "person_limit") })
	t.Run("step", func(t *testing.T) { pinGolden[Step](t, "step") })
	t.Run("message", func(t *testing.T) { pinGolden[Message](t, "message") })
	t.Run("parts", func(t *testing.T) { pinGolden[Parts](t, "parts") })
	t.Run("limit", func(t *testing.T) { pinGolden[Limit](t, "limit") })
	t.Run("result", func(t *testing.T) { pinGolden[Result](t, "result") })
	t.Run("watchlist_item", func(t *testing.T) { pinGolden[WatchlistItem](t, "watchlist_item") })
	t.Run("watchlist_state", func(t *testing.T) { pinGolden[WatchlistState](t, "watchlist_state") })
	t.Run("domain_ask", func(t *testing.T) { pinGolden[domain.Ask](t, "domain_ask") })
	t.Run("domain_report", func(t *testing.T) { pinGolden[domain.Report](t, "domain_report") })
	t.Run("domain_outside_list", func(t *testing.T) { pinGolden[domain.OutsideList](t, "domain_outside_list") })
	t.Run("domain_ask_episodes", func(t *testing.T) { pinGolden[domain.AskEpisodes](t, "domain_ask_episodes") })
	t.Run("domain_ask_album", func(t *testing.T) { pinGolden[domain.AskAlbum](t, "domain_ask_album") })
}
