package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Every metadata call and the Deezer and bio reads reach the music module;
// a nil one would quietly hide rows on Explore and the artist pages.
func TestMainWiresEveryMusicDep(t *testing.T) {
	deezer := func(context.Context, string) (metadata.DeezerProfile, error) { return metadata.DeezerProfile{}, nil }
	album := func(context.Context, int64) (metadata.DeezerAlbumInfo, error) { return metadata.DeezerAlbumInfo{}, nil }
	bio := func(context.Context, string) (*metadata.WikiSummary, error) { return nil, nil }
	d := musicDeps(&metadata.Service{}, deezer, album, bio)
	v := reflect.ValueOf(d)
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Func && f.IsNil() {
			t.Errorf("music.Deps.%s is not wired", v.Type().Field(i).Name)
		}
	}
	if d.MusicRowsTplOff {
		t.Fatalf("switches do not follow HOLYARRS_FLAGS: %+v", d)
	}
}

// The health card is built from the Manager's own state, with the key and
// key-kit fingerprints in the right fields.
func TestBackupsHealthFromManager(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	bm := &backups.Manager{Store: s, Dir: dir, Version: "test", Key: key}
	ctx := context.Background()
	if _, err := bm.Create(ctx, backups.KindManual); err != nil {
		t.Fatal(err)
	}
	h, err := backupsHealth(bm)(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(h.Newest) > time.Minute || h.Newest.After(time.Now().Add(time.Minute)) {
		t.Fatalf("newest backup %v", h.Newest)
	}
	if h.Every <= 0 {
		t.Fatalf("every %v", h.Every)
	}
	if h.Started != bm.Passed() {
		t.Fatalf("started %v", h.Started)
	}
	if h.KeyPrint == "" || h.KeyPrint != bm.KeyPrint() {
		t.Fatalf("key print %q", h.KeyPrint)
	}
	if h.KeyKitPrint != "" {
		t.Fatalf("key kit print %q before any download", h.KeyKitPrint)
	}

	// Downloading the key kit fills in KeyKitPrint, equal to KeyPrint.
	mux := http.NewServeMux()
	bm.Routes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", backups.KeyKitPath, nil))
	if rec.Code != 200 {
		t.Fatalf("key kit: %d %s", rec.Code, rec.Body)
	}
	h, err = backupsHealth(bm)(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if h.KeyKitPrint == "" || h.KeyKitPrint != h.KeyPrint {
		t.Fatalf("after download: key %q kit %q", h.KeyPrint, h.KeyKitPrint)
	}
}

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
