package downloads

import (
	"testing"
	"time"
)

// A torrent qBittorrent just finished in its temporary folder is moved to the
// save folder next; importing before then reads a path that is about to go.
func TestQbitFinishedInTempFolderWaitsForTheMove(t *testing.T) {
	now := time.Now().Unix()
	base := qbitTorrent{Hash: "AB", Name: "Movie.mkv", State: "stalledUP", Progress: 1,
		SavePath: "/data/torrents/movies", DownloadPath: "/downloads/qbit/incomplete", ContentPath: "/downloads/qbit/incomplete/radarr/Movie.mkv"}

	just := base
	just.DoneOn = now - 5
	if j := jobFromTorrent(just); j.State != StateQueued || j.Detail != "moving" {
		t.Errorf("just finished in the temporary folder: %s (%s), want queued while moving", j.State, j.Detail)
	}

	unknown := base // no completion time from the client: never wait, or it could wait forever
	if j := jobFromTorrent(unknown); j.State != StateFinished {
		t.Errorf("no completion time: %s, want finished", j.State)
	}

	stuck := base
	stuck.DoneOn = now - int64(moveWait/time.Second) - 60
	if j := jobFromTorrent(stuck); j.State != StateFinished || j.SavePath != "/downloads/qbit/incomplete" {
		t.Errorf("never moved: %s in %s, want finished in the temporary folder", j.State, j.SavePath)
	}

	moved := base
	moved.DoneOn = now - 5
	moved.ContentPath = "/data/torrents/movies/Movie.mkv"
	if j := jobFromTorrent(moved); j.State != StateFinished || j.SavePath != "/data/torrents/movies" {
		t.Errorf("moved: %s in %s, want finished in the save folder", j.State, j.SavePath)
	}
}
