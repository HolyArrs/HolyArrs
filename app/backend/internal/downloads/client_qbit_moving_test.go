package downloads

import "testing"

func TestQbitCompleteButStillInIncompleteFolder(t *testing.T) {
	for _, c := range []struct {
		name, save, download, content, want string
	}{
		{"file, incomplete folder holds the save folder", "/data/inc/holyarrs", "/data/inc", "/data/inc/Dune.mkv", "/data/inc"},
		{"subfolder layout, single file", "/data/inc/holyarrs", "/data/inc", "/data/inc/Dune/Dune.mkv", "/data/inc"},
		{"renamed", "/data/inc/holyarrs", "/data/inc", "/data/inc/Renamed.2021.mkv", "/data/inc"},
		{"incomplete folder inside the save folder", "/downloads", "/downloads/incomplete", "/downloads/incomplete/Dune.mkv", "/downloads/incomplete"},
		{"moved", "/data/inc/holyarrs", "/data/inc", "/data/inc/holyarrs/Dune.mkv", "/data/inc/holyarrs"},
		{"moved out of an inner incomplete folder", "/downloads", "/downloads/incomplete", "/downloads/Dune.mkv", "/downloads"},
		{"same folder", "/downloads", "/downloads", "/downloads/Dune.mkv", "/downloads"},
		{"no incomplete folder", "/data/inc/holyarrs", "", "/data/inc/Dune.mkv", "/data/inc/holyarrs"},
		{"windows paths", `D:\inc\holyarrs`, `D:\inc`, `D:\inc\Dune.mkv`, `D:\inc`},
	} {
		tt := qbitTorrent{Hash: "AB", Name: "Dune.mkv", State: "stalledUP", Progress: 1, SavePath: c.save, DownloadPath: c.download, ContentPath: c.content}
		j := jobFromTorrent(tt)
		if j.State != StateFinished || j.SavePath != c.want {
			t.Errorf("%s: %s in %s, want finished in %s", c.name, j.State, j.SavePath, c.want)
		}
	}
	dl := qbitTorrent{Hash: "AB", Name: "Dune.mkv", State: "downloading", Progress: 0.5, SavePath: "/data/inc/holyarrs", DownloadPath: "/data/inc", ContentPath: "/data/inc/Dune.mkv"}
	if j := jobFromTorrent(dl); j.State != StateDownloading || j.SavePath != dl.SavePath {
		t.Errorf("downloading: %s in %s", j.State, j.SavePath)
	}
}
