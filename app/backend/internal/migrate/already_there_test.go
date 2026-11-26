package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func alreadyRun(t *testing.T, l *moveLab) *run {
	ctx := context.Background()
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
	return &run{m: l.mv, ctx: ctx, move: id, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
}

// A Downloader the owner added by hand under another name is the one the
// import finds: no "(imported)" copy, the owner's settings stay, and only
// an empty user name and empty folder maps are filled.
func TestImportKeepsDownloaderAlreadyThere(t *testing.T) {
	l := newMoveLab(t)
	r := alreadyRun(t, l)
	if err := l.st.Write(r.ctx, func(tx *sql.Tx) error {
		row := domain.Downloader{Name: "My SAB", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://10.1.1.4:8080", Category: "mine", Priority: 5, AfterImport: "leave", State: "ok"}
		_, err := database.Insert(tx, "downloaders", &row, r.by)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	it := Item{Key: "downloader:http://10.1.1.4:8080", Name: "SABnzbd", Downloader: &DownloaderPlan{Adapter: "sabnzbd", URL: "http://10.1.1.4:8080/", Username: "olduser", Priority: 1}}
	got, why := r.one(GroupDownloaders, it)
	if got != ResAlready || !strings.Contains(why, "already there, kept") {
		t.Fatalf("%s: %s", got, why)
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders"); n != 1 {
		t.Fatalf("Downloaders: %d, want 1", n)
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE name = 'My SAB' AND category = 'mine' AND priority = 5 AND username = 'olduser'"); n != 1 {
		t.Fatal("kept Downloader lost its settings or the empty user name was not filled")
	}
}

// A Source at the same address in the same format is the one already
// here, whatever its name.
func TestImportKeepsSourceAlreadyThere(t *testing.T) {
	l := newMoveLab(t)
	r := alreadyRun(t, l)
	first := Item{Key: "source:a", Name: "NZBgeek", Source: &SourcePlan{Format: "newznab", Protocol: "usenet", URL: "https://api.nzbgeek.info", Priority: 25}}
	if got, why := r.one(GroupSources, first); got != ResCreated {
		t.Fatalf("first %s: %s", got, why)
	}
	again := Item{Key: "source:b", Name: "NZBgeek (Prowlarr)", Source: &SourcePlan{Format: "newznab", Protocol: "usenet", URL: "https://api.nzbgeek.info/", Priority: 25}}
	if got, why := r.one(GroupSources, again); got != ResAlready || !strings.Contains(why, "already there, kept") {
		t.Fatalf("same address %s: %s", got, why)
	}
	if n := l.count(t, "SELECT count(*) FROM sources"); n != 1 {
		t.Fatalf("Sources: %d, want 1", n)
	}
}

// An Alert of the same kind writing to the same place is the one already
// here, whatever its name: a Telegram chat, or mail with the same server,
// sender and recipients.
func TestImportKeepsAlertAlreadyThere(t *testing.T) {
	l := newMoveLab(t)
	r := alreadyRun(t, l)
	first := Item{Key: "alert:a", Name: "mail", Needs: "x", Alert: &AlertPlan{Kind: "email", SMTPHost: "smtp.example.com", SMTPFrom: "tv@example.com", StaffTo: "me@example.com"}}
	if got, why := r.one(GroupAlerts, first); got != ResCreated {
		t.Fatalf("first %s: %s", got, why)
	}
	again := Item{Key: "alert:b", Name: "mail from Radarr", Needs: "x", Alert: &AlertPlan{Kind: "email", SMTPHost: "SMTP.example.com", SMTPFrom: "tv@example.com", StaffTo: "me@example.com"}}
	if got, why := r.one(GroupAlerts, again); got != ResAlready || !strings.Contains(why, "already there, kept") {
		t.Fatalf("same place %s: %s", got, why)
	}
}

// Alerts that share only a server still go to different places (the ntfy
// topic or Gotify app is not imported; mail to other people): each is added.
func TestImportAddsAlertsSharingOnlyAServer(t *testing.T) {
	l := newMoveLab(t)
	r := alreadyRun(t, l)
	for i, it := range []Item{
		{Key: "alert:tv", Name: "ntfy tv", Needs: "x", Alert: &AlertPlan{Kind: "ntfy", ServerURL: "https://ntfy.sh"}},
		{Key: "alert:movies", Name: "ntfy movies", Needs: "x", Alert: &AlertPlan{Kind: "ntfy", ServerURL: "https://ntfy.sh"}},
		{Key: "alert:g1", Name: "gotify a", Needs: "x", Alert: &AlertPlan{Kind: "gotify", ServerURL: "https://g.example.com"}},
		{Key: "alert:g2", Name: "gotify b", Needs: "x", Alert: &AlertPlan{Kind: "gotify", ServerURL: "https://g.example.com"}},
		{Key: "alert:m1", Name: "mail a", Needs: "x", Alert: &AlertPlan{Kind: "email", SMTPHost: "smtp.example.com", SMTPFrom: "a@example.com", StaffTo: "one@example.com"}},
		{Key: "alert:m2", Name: "mail b", Needs: "x", Alert: &AlertPlan{Kind: "email", SMTPHost: "smtp.example.com", SMTPFrom: "a@example.com", StaffTo: "two@example.com"}},
	} {
		if got, why := r.one(GroupAlerts, it); got != ResCreated {
			t.Fatalf("%d %s: %s: %s", i, it.Name, got, why)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM alerts"); n != 6 {
		t.Fatalf("Alerts: %d, want 6", n)
	}
}
