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
	t.Setenv("HOLYARRS_FLAGS", "music_rows_v7")
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
	if !d.MusicRowsV7 || d.MusicRowsV2 || d.MusicRowsV3 || d.MusicRowsV6 || d.MusicListsV1 || d.MusicRowsTplOff {
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
