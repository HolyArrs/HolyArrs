package requests

import (
	"bufio"
	"context"
	"database/sql"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

type gotMail struct {
	to   []string
	data string
}

// mailCatcher is a tiny SMTP server on 127.0.0.1 that keeps each message.
func mailCatcher(t *testing.T) (int, func() []gotMail) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var got []gotMail
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				say := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
				say("220 fake")
				var m gotMail
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "RCPT TO:"):
						m.to = append(m.to, strings.Trim(strings.TrimSpace(line)[8:], "<>"))
						say("250 ok")
					case cmd == "DATA":
						say("354 go on")
						var b strings.Builder
						for {
							l, err := r.ReadString('\n')
							if err != nil || l == ".\r\n" {
								break
							}
							b.WriteString(l)
						}
						m.data = b.String()
						mu.Lock()
						got = append(got, m)
						mu.Unlock()
						m = gotMail{}
						say("250 queued")
					case cmd == "QUIT":
						say("221 bye")
						return
					default:
						say("250 ok")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() []gotMail {
		mu.Lock()
		defer mu.Unlock()
		return append([]gotMail(nil), got...)
	}
}

func header(data, name string) string {
	for _, l := range strings.Split(data, "\r\n") {
		if strings.HasPrefix(l, name+": ") {
			return strings.TrimPrefix(l, name+": ")
		}
	}
	return ""
}

func body(data string) string {
	_, b, _ := strings.Cut(data, "\r\n\r\n")
	return strings.TrimSpace(b)
}

// mailRig is newRig with a real email Alert pointing at a local catcher.
func mailRig(t *testing.T, moments []string) (*rig, Person, func() []gotMail) {
	t.Helper()
	outbound.Reset()
	t.Cleanup(outbound.Reset)
	r := newRig(t)
	ctx := context.Background()
	v, err := secrets.Open(ctx, r.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	port, mails := mailCatcher(t)
	yes := true
	if _, err := alerts.Add(ctx, r.st, v, alerts.Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port,
		SMTPSecurity: "none", SMTPFrom: "holyarrs@lab.test", Moments: moments, AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	r.svc.Alerts = &alerts.Sender{Store: r.st, Vault: v, Backoff: []time.Duration{}}
	var cy int64
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		cy, err = database.Insert(tx, "users", &domain.User{Name: "cy", Role: "member", State: "active"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// My account: ana and bo give an address, cy gives none.
	for id, e := range map[int64]string{r.ana.UserID: "ana@lab.test", r.bo.UserID: "bo@lab.test"} {
		if err := SaveTargets(ctx, r.st, id, Targets{Email: e}); err != nil {
			t.Fatal(err)
		}
	}
	return r, Person{UserID: cy, Name: "cy"}, mails
}

func placeInLibrary(t *testing.T, r *rig, titleID int64) {
	t.Helper()
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", titleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func mailsTo(ms []gotMail, addr string) []gotMail {
	var out []gotMail
	for _, m := range ms {
		for _, x := range m.to {
			if x == addr {
				out = append(out, m)
			}
		}
	}
	return out
}

func TestReadyEmailReachesEachRequesterOnly(t *testing.T) {
	r, cy, mails := mailRig(t, []string{alerts.MomentReady})
	ctx := context.Background()
	want := map[string]string{"ana@lab.test": "Dune: Part Two (2024)", "bo@lab.test": "Paper Harbour (2023)"}
	var titles []int64
	for _, c := range []struct {
		p    Person
		tmdb string
	}{{r.ana, "693134"}, {r.bo, "900002"}, {cy, "900003"}} {
		res, err := r.svc.Ask(ctx, c.p, c.tmdb)
		if err != nil {
			t.Fatal(err)
		}
		a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		titles = append(titles, *a.TitleID)
	}
	r.svc.Wait()
	if n := len(mails()); n != 0 {
		t.Fatalf("Approved not ticked, yet %d mails", n)
	}
	for _, id := range titles {
		placeInLibrary(t, r, id)
	}
	r.clock = r.clock.Add(time.Hour)
	if n, err := r.svc.CheckReady(ctx); n != 3 || err != nil {
		t.Fatalf("ready %d %v", n, err)
	}
	_, _ = r.svc.CheckReady(ctx)
	r.svc.Wait()
	all := mails()
	if len(all) != 2 {
		t.Fatalf("%d mails, want 2: %+v", len(all), all)
	}
	for addr, title := range want {
		got := mailsTo(all, addr)
		if len(got) != 1 || len(got[0].to) != 1 {
			t.Fatalf("%s got %+v", addr, got)
		}
		subj, b := header(got[0].data, "Subject"), body(got[0].data)
		if subj != "Ready to watch: "+title || b != title+" is in the Library and ready to watch." {
			t.Fatalf("%s: subject %q body %q", addr, subj, b)
		}
		for other, ot := range want {
			if other != addr && (strings.Contains(got[0].data, ot) || strings.Contains(got[0].data, other)) {
				t.Fatalf("%s saw %s's title or address", addr, other)
			}
		}
		t.Logf("to=%s subject=%q body=%q", addr, subj, b)
	}
}

func TestApprovedAndTurnedDownEmails(t *testing.T) {
	r, cy, mails := mailRig(t, []string{alerts.MomentApproved, alerts.MomentTurnedDown})
	ctx := context.Background()
	a1, _ := r.svc.Ask(ctx, r.ana, "693134")
	b1, _ := r.svc.Ask(ctx, r.bo, "900002")
	c1, _ := r.svc.Ask(ctx, cy, "900003")
	if _, err := r.svc.Approve(ctx, a1.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.TurnDown(ctx, b1.Ask.ID, r.luis, "we have it on disc"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, c1.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	all := mails()
	if len(all) != 2 {
		t.Fatalf("%d mails, want 2: %+v", len(all), all)
	}
	for addr, word := range map[string]string{"ana@lab.test": "Dune", "bo@lab.test": "Paper Harbour"} {
		got := mailsTo(all, addr)
		if len(got) != 1 || !strings.Contains(got[0].data, word) {
			t.Fatalf("%s got %+v", addr, got)
		}
		t.Logf("to=%s subject=%q body=%q", addr, header(got[0].data, "Subject"), body(got[0].data))
	}
}

func TestReadyEmailSharedAskKeepsAddressesApart(t *testing.T) {
	r, _, mails := mailRig(t, []string{alerts.MomentReady})
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	placeInLibrary(t, r, *a.TitleID)
	r.clock = r.clock.Add(time.Hour)
	_, _ = r.svc.CheckReady(ctx)
	r.svc.Wait()
	for _, m := range mails() {
		t.Logf("rcpt=%v To=%q", m.to, header(m.data, "To"))
		if len(m.to) != 1 || strings.Contains(m.data, "ana@lab.test") && strings.Contains(m.data, "bo@lab.test") {
			t.Fatalf("one mail shows both requesters' addresses: %v", m.to)
		}
	}
	if len(mails()) != 2 {
		t.Fatalf("%d mails, want 2", len(mails()))
	}
}
