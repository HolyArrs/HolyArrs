package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A path pair with the same folder on both sides changes nothing, so it is
// not stored; other pairs are kept as typed.
func TestSamePathPairIsNotStored(t *testing.T) {
	in := []domain.PathMap{
		{Theirs: "/mnt/Torrents", Ours: "/mnt/Torrents"},
		{Theirs: "/mnt/Torrents/", Ours: "/mnt/Torrents"},
		{Theirs: " /data ", Ours: "/data//"},
		{Theirs: `D:\Torrents\`, Ours: `d:/torrents`},
		{Theirs: "/downloads", Ours: "/data/downloads"},
		{Theirs: "/mnt/torrents", Ours: "/mnt/Torrents"}, // Linux: case counts
	}
	var d domain.Downloader
	if err := fill(Input{Name: "q", Adapter: "qbittorrent", URL: "http://q:8080", PathMap: in}, &d, true); err != nil {
		t.Fatal(err)
	}
	if len(d.PathMap) != 2 || d.PathMap[0].Theirs != "/downloads" || d.PathMap[1].Theirs != "/mnt/torrents" {
		t.Fatalf("stored %+v", d.PathMap)
	}
	var e domain.Downloader
	if err := fill(Input{Name: "q", Adapter: "qbittorrent", URL: "http://q:8080", PathMap: in[:1]}, &e, true); err != nil || e.PathMap == nil || len(e.PathMap) != 0 {
		t.Fatalf("only a same pair: %+v %v", e.PathMap, err)
	}
}
