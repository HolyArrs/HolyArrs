package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Tags on Downloaders, Sources and titles: cleaned on save, kept
// when not sent, cleared with an empty list, and listed by GET /tags.
func TestTagsOnDownloadersSourcesAndTitles(t *testing.T) {
	t.Parallel()
	srv, s := lbDlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab := lbFakeSAB(t, key)
	var d downloads.DownloaderView
	raw, _ := json.Marshal(map[string]any{"name": "Kids SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key, "category": "movies",
		"allow_local_network": true, "tags": []string{" Kids ", "kids", "4K  Only"}})
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 201), &d)
	if !reflect.DeepEqual(d.Tags, []string{"4k-only", "kids"}) {
		t.Fatalf("downloader tags %v", d.Tags)
	}
	// Not sent: kept. Empty list: cleared.
	_ = json.Unmarshal(lbDo(t, "PUT", srv.URL+"/api/v1/downloaders/"+lbItoa(d.ID), `{"name":"Kids SAB","kind":"sabnzbd","url":"`+sab.URL+`","category":"movies","allow_local_network":true}`, 200), &d)
	if !reflect.DeepEqual(d.Tags, []string{"4k-only", "kids"}) {
		t.Fatalf("after an edit without tags %v", d.Tags)
	}

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title></channel></rss>`))
	}))
	t.Cleanup(feed.Close)
	var src sources.SourceView
	raw, _ = json.Marshal(map[string]any{"name": "Kids feed", "format": "rss", "url": feed.URL, "allow_local_network": true, "tags": []string{"Kids"}})
	_ = json.Unmarshal(lbDo(t, "POST", srv.URL+"/api/v1/sources", string(raw), 201), &src)
	if !reflect.DeepEqual(src.Tags, []string{"kids"}) {
		t.Fatalf("source tags %v", src.Tags)
	}

	var titleID int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Lantern", SortName: "lantern", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand",
			Artwork: []string{}, Status: "released", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		titleID = ti.ID
		// A title saved without tags holds JSON null: GET /tags must skip it.
		other := *ti
		other.ID, other.Name, other.SortName, other.Tags = 0, "Untagged", "untagged", nil
		_, err := database.Insert(tx, "titles", &other, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var edited struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(lbDo(t, "PUT", srv.URL+"/api/v1/titles/"+lbItoa(titleID), `{"tags":["Kids","Family Night"]}`, 200), &edited)
	if !reflect.DeepEqual(edited.Title.Tags, []string{"family-night", "kids"}) {
		t.Fatalf("title tags %v", edited.Title.Tags)
	}

	var list struct {
		Tags []downloads.TagUse `json:"tags"`
	}
	_ = json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/tags", "", 200), &list)
	want := []downloads.TagUse{{Label: "4k-only", Downloaders: 1}, {Label: "family-night", Titles: 1}, {Label: "kids", Titles: 1, Downloaders: 1, Sources: 1}}
	if !reflect.DeepEqual(list.Tags, want) {
		t.Fatalf("tags %+v", list.Tags)
	}

	_ = json.Unmarshal(lbDo(t, "PUT", srv.URL+"/api/v1/downloaders/"+lbItoa(d.ID), `{"name":"Kids SAB","kind":"sabnzbd","url":"`+sab.URL+`","category":"movies","allow_local_network":true,"tags":[]}`, 200), &d)
	if len(d.Tags) != 0 {
		t.Fatalf("cleared tags %v", d.Tags)
	}
}
