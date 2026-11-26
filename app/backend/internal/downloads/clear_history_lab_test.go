//go:build lab

package downloads

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

// Remove after import clears a finished job and its files from the client.
// The lab has no news server: an NZBGet job is sent to history and cleared
// from there; a SABnzbd job is cleared through the same queue and history
// calls. Either way the client no longer holds it afterwards.
func TestLabClearHistoryJob(t *testing.T) {
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	nzb := `<?xml version="1.0" encoding="utf-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"><file poster="lab@example.invalid" date="1" subject="HolyArrs clear &quot;clear.bin&quot; yEnc (1/1)">
<groups><group>alt.binaries.test</group></groups><segments><segment bytes="1000" number="1">clear-` + stamp + `@example.invalid</segment></segments></file></nzb>`
	clients := map[string]func(t *testing.T) (Engine, string){
		"sabnzbd": func(t *testing.T) (Engine, string) {
			base, key := os.Getenv("HOLYARRS_LAB_SAB"), os.Getenv("HOLYARRS_LAB_SAB_KEY")
			if base == "" || key == "" {
				t.Skip("HOLYARRS_LAB_SAB not set")
			}
			s := NewSabnzbd(base, key, nil)
			ctx := context.Background()
			name := "HolyArrs.Clear.Lab." + stamp
			id, err := s.Add(ctx, AddRequest{File: []byte(nzb), Name: name, Category: "*"})
			if err != nil {
				t.Fatalf("add: %v", err)
			}
			// No news server: the job waits in the queue; Remove asks the queue
			// and the history, so both calls meet this SABnzbd build.
			return s, id
		},
		"nzbget": func(t *testing.T) (Engine, string) {
			base := os.Getenv("HOLYARRS_LAB_NZBGET_URL")
			if base == "" {
				t.Skip("HOLYARRS_LAB_NZBGET_URL not set")
			}
			n := NewNZBGet(base, os.Getenv("HOLYARRS_LAB_NZBGET_USER"), os.Getenv("HOLYARRS_LAB_NZBGET_PASS"), nil)
			ctx := context.Background()
			var nzbID int
			if err := n.rpc(ctx, "append", []any{"HolyArrs.Clear.Lab." + stamp + ".nzb", base64.StdEncoding.EncodeToString([]byte(nzb)), "", 0, false, true, "", 0, "ALL", false, [][]string{}}, &nzbID); err != nil || nzbID <= 0 {
				t.Fatalf("append %d %v", nzbID, err)
			}
			var ok bool
			if err := n.rpc(ctx, "editqueue", []any{"GroupDelete", "", []int{nzbID}}, &ok); err != nil || !ok {
				t.Fatalf("delete into history %v", err)
			}
			return n, "nzbid-" + strconv.Itoa(nzbID)
		},
	}
	for name, open := range clients {
		t.Run(name, func(t *testing.T) {
			e, id := open(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if name == "nzbget" {
				// Wait until the job sits in history.
				for range 20 {
					if j, err := e.Get(ctx, id); err == nil && j.State == StateFailed {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
			}
			if err := e.Remove(ctx, id, true); err != nil && !(name == "sabnzbd" && errors.Is(err, ErrNotFound)) {
				t.Fatalf("remove: %v", err)
			}
			for range 20 {
				if _, err := e.Get(ctx, id); errors.Is(err, ErrNotFound) {
					return
				}
				time.Sleep(250 * time.Millisecond)
			}
			t.Fatalf("%s still holds %s after the clear", name, id)
		})
	}
}
