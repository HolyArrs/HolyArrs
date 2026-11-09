package migrate

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
	if depth > 9 {
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

// The records the move tool saves and returns keep their JSON field names (R4).
func TestPinMigrateRecordJSON(t *testing.T) {
	t.Run("snapshot", func(t *testing.T) { pinGolden[Snapshot](t, "pin_snapshot") })
	t.Run("saved", func(t *testing.T) { pinGolden[Saved](t, "pin_saved") })
	t.Run("plan", func(t *testing.T) { pinGolden[Plan](t, "pin_plan") })
	t.Run("report", func(t *testing.T) { pinGolden[Report](t, "pin_report") })
	t.Run("move", func(t *testing.T) { pinGolden[Move](t, "pin_move") })
	t.Run("choices", func(t *testing.T) { pinGolden[Choices](t, "pin_choices") })
	t.Run("input", func(t *testing.T) { pinGolden[Input](t, "pin_input") })
	t.Run("info", func(t *testing.T) { pinGolden[Info](t, "pin_info") })
	t.Run("source", func(t *testing.T) { pinGolden[Source](t, "pin_source") })
	t.Run("line", func(t *testing.T) { pinGolden[Line](t, "pin_line") })
}
