//go:build lab

package downloads

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const (
	delugeLabCategory = "holyarrs-labtest"
	delugeLabFile     = "holyarrs-deluge-acceptance.txt"
)

var delugeLabPayload = []byte("HolyArrs disposable Deluge acceptance\n")

func TestLabDeluge(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_DELUGE")
	password := os.Getenv("HOLYARRS_LAB_DELUGE_PASSWORD")
	if path := os.Getenv("HOLYARRS_LAB_DELUGE_PASSWORD_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil || len(b) == 0 || len(b) > 4096 {
			t.Fatal("read disposable Deluge password file failed")
		}
		password = string(b)
	}
	if base == "" || password == "" {
		t.Skip("set HOLYARRS_LAB_DELUGE and HOLYARRS_LAB_DELUGE_PASSWORD_FILE")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	d := NewDeluge(base, password, &http.Client{Timeout: 10 * time.Second})
	d.Category = delugeLabCategory

	health, err := d.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Version == "" {
		t.Fatal("Health returned an empty version")
	}

	torrent := delugeLabTorrent()
	hash, err := torrentFileHash(torrent)
	if err != nil {
		t.Fatalf("torrent hash: %v", err)
	}
	if expected := os.Getenv("HOLYARRS_LAB_DELUGE_EXPECT_HASH"); expected != "" && hash != expected {
		t.Fatal("disposable torrent fixture does not match the expected hash")
	}
	_, getErr := d.Get(ctx, hash)
	existed := getErr == nil
	if getErr != nil && !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("initial Get: %v", getErr)
	}
	if os.Getenv("HOLYARRS_LAB_DELUGE_EXPECT_EXISTING") == "1" && !existed {
		t.Fatal("torrent did not persist across the container restart")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(torrent)
	}))
	defer server.Close()
	id, err := d.Add(ctx, AddRequest{
		URL:        server.URL + "/acceptance.torrent",
		Name:       delugeLabFile,
		Category:   delugeLabCategory,
		SavePath:   "/downloads",
		InfoHash:   hash,
		AllowLocal: true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != hash {
		t.Fatalf("Add id = %q, want %q", id, hash)
	}
	if got, err := d.Add(ctx, AddRequest{InfoHash: hash, Category: delugeLabCategory, ReconcileOnly: true}); err != nil || got != hash {
		t.Fatal("saved-hash reconciliation failed")
	}

	jobs, err := d.List(ctx, delugeLabCategory)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, job := range jobs {
		found = found || job.ID == hash
	}
	if !found {
		t.Fatalf("List did not include %s", hash)
	}

	job, err := d.Get(ctx, hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.Category != delugeLabCategory || len(job.Files) != 1 || job.Files[0].Name != delugeLabFile {
		t.Fatalf("Get metadata = category %q files %#v", job.Category, job.Files)
	}
	if os.Getenv("HOLYARRS_LAB_DELUGE_EXPECT_FINISHED") == "1" {
		deadline := time.Now().Add(20 * time.Second)
		for job.State != StateFinished && time.Now().Before(deadline) {
			time.Sleep(250 * time.Millisecond)
			job, err = d.Get(ctx, hash)
			if err != nil {
				t.Fatalf("Get while waiting for completion: %v", err)
			}
		}
		if job.State != StateFinished || job.Progress != 1 {
			t.Fatalf("completed local payload state = %q progress %.3f", job.State, job.Progress)
		}
	}

	if os.Getenv("HOLYARRS_LAB_DELUGE_KEEP") == "1" {
		t.Logf("kept acceptance torrent %s for restart verification", hash)
		return
	}
	if err := d.Remove(ctx, hash, false); err != nil {
		t.Fatalf("Remove preserving payload: %v", err)
	}
	if _, err := d.Get(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Remove = %v, want ErrNotFound", err)
	}
	t.Logf("Deluge %s accepted uploaded torrent %s; existed=%t; final_state=%s", health.Version, hash, existed, job.State)
}

func bencodeString(value []byte) []byte {
	return append([]byte(fmt.Sprintf("%d:", len(value))), value...)
}

func delugeLabTorrent() []byte {
	piece := sha1.Sum(delugeLabPayload)
	info := []byte("d6:lengthi" + fmt.Sprint(len(delugeLabPayload)) + "e4:name")
	info = append(info, bencodeString([]byte(delugeLabFile))...)
	info = append(info, []byte("12:piece lengthi16384e6:pieces")...)
	info = append(info, bencodeString(piece[:])...)
	info = append(info, 'e')

	torrent := []byte("d8:announce")
	torrent = append(torrent, bencodeString([]byte("http://127.0.0.1:9/announce"))...)
	torrent = append(torrent, []byte("4:info")...)
	torrent = append(torrent, info...)
	torrent = append(torrent, 'e')
	return torrent
}
