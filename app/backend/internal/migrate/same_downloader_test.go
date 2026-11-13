package migrate

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// An old app's SABnzbd at http://host:port/sabnzbd is the SABnzbd already
// added at http://host:port: the import reuses it instead of adding a
// second Downloader (and a "(imported)" twin).
func TestImportReusesSameDownloaderAtAnotherPath(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	r := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	first := Item{Key: "downloader:http://192.168.1.124:30055", Name: "SABnzbd", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://192.168.1.124:30055", Priority: 1}}
	if got, why := r.one(GroupDownloaders, first); got != ResCreated {
		t.Fatalf("first SABnzbd %s: %s", got, why)
	}
	again := Item{Key: "downloader:http://192.168.1.124:30055/sabnzbd", Name: "SABnzbd", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://192.168.1.124:30055/sabnzbd", Priority: 1}}
	if got, why := r.one(GroupDownloaders, again); got != ResAlready {
		t.Fatalf("same SABnzbd at /sabnzbd %s: %s", got, why)
	}
	other := Item{Key: "downloader:http://192.168.1.124:30056", Name: "SABnzbd", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://192.168.1.124:30056", Priority: 1}}
	if got, why := r.one(GroupDownloaders, other); got != ResCreated {
		t.Fatalf("another port is another SABnzbd %s: %s", got, why)
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE adapter = 'sabnzbd'"); n != 2 {
		t.Fatalf("SABnzbd Downloaders: %d, want 2", n)
	}
}
