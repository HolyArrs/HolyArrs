package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// rdpFill sets every exported field to a fixed non-zero value, so a
// marshalled record shows all of its JSON names, omitempty ones included.
func rdpFill(v reflect.Value, depth int) {
	if depth > 4 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		rdpFill(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				rdpFill(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		rdpFill(s.Index(0), depth+1)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		rdpFill(k, depth+1)
		e := reflect.New(v.Type().Elem()).Elem()
		rdpFill(e, depth+1)
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

// rdpGolden compares a filled T to testdata/<name>.golden.json (PIN_WRITE=1 writes it).
func rdpGolden[T any](t *testing.T, name string) {
	t.Helper()
	var v T
	rdpFill(reflect.ValueOf(&v).Elem(), 0)
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

// The records the requests and discover routes return keep their JSON names (R4).
func TestRDPRecordJSON(t *testing.T) {
	t.Run("discover_item_view", func(t *testing.T) { rdpGolden[discover.ItemView](t, "rdp_discover_item_view") })
	t.Run("ask_brief", func(t *testing.T) { rdpGolden[askBrief](t, "rdp_ask_brief") })
	t.Run("asked_by_entry", func(t *testing.T) { rdpGolden[requests.AskedByEntry](t, "rdp_asked_by_entry") })
	t.Run("ask_limit_view", func(t *testing.T) { rdpGolden[requests.AskLimitView](t, "rdp_ask_limit_view") })
	t.Run("ask_view", func(t *testing.T) { rdpGolden[requests.View](t, "rdp_ask_view") })
}
