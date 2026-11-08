package main

import (
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// optionalDeps are funcs a module checks for nil on purpose: main.go sets
// them only when the service exists (anime map, restart, media servers).
var optionalDeps = map[string]bool{
	"shows.AnimeMap":           true,
	"controls.ExitApp":         true,
	"controls.TestMediaServer": true,
}

// TestNoNilDepsFunc builds the handlers the way main.go does (two phases:
// every module first, then the funcs that point upward) and fails when a
// module's Deps still holds a nil func, which would panic on first use.
func TestNoNilDepsFunc(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := buildHandlers(services{Store: st})
	h.mh() // music is built on first use
	v := reflect.ValueOf(h).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() || f.Elem().Kind() != reflect.Struct {
			continue
		}
		deps := f.Elem().FieldByName("Deps")
		if !deps.IsValid() || deps.Kind() != reflect.Struct {
			continue
		}
		for j := 0; j < deps.NumField(); j++ {
			name := v.Type().Field(i).Name + "." + deps.Type().Field(j).Name
			if d := deps.Field(j); d.Kind() == reflect.Func && d.IsNil() && !optionalDeps[name] {
				t.Errorf("%s is nil in Deps", name)
			}
		}
	}
}
