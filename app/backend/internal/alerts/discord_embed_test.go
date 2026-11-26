package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const pictures = "https://api.holyarrs.example/v1/img"

func setPoster(t *testing.T, s *database.Store, id int64, url string) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET poster_url = ? WHERE id = ?", url, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// embedOf decodes a Discord body and returns its content and single embed.
func embedOf(t *testing.T, body []byte) (map[string]any, string, map[string]any) {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	es, _ := p["embeds"].([]any)
	if len(es) != 1 {
		t.Fatalf("embeds %s", body)
	}
	c, _ := p["content"].(string)
	return p, c, es[0].(map[string]any)
}

// A Discord message is one embed: the subject as title, the text as
// description, a colour and the poster as thumbnail when it is a public
// https picture. Mentions stay in content, and only for people moments.
func TestDiscordBodyShape(t *testing.T) {
	ana := []Person{{Name: "ana", DiscordID: "123456789012"}, {Name: "bo", DiscordID: "x"}}
	for _, c := range []struct {
		name      string
		m         Message
		content   string
		thumb     string
		mentioned []string
	}{
		{"movie import with poster", Message{Moment: MomentImport, Subject: "In the Library: Arrival (2016)", Text: "Import done", Poster: "https://image.tmdb.org/t/p/w342/a.jpg"},
			"", "https://image.tmdb.org/t/p/w342/a.jpg", nil},
		{"no poster", Message{Moment: MomentImport, Subject: "In the Library: Arrival (2016)", Text: "x"}, "", "", nil},
		{"local poster is never sent", Message{Moment: MomentImport, Subject: "s", Text: "x", Poster: "/api/v1/artwork/abc.jpg"}, "", "", nil},
		{"plain http poster is never sent", Message{Moment: MomentImport, Subject: "s", Text: "x", Poster: "http://10.0.0.5/p.jpg"}, "", "", nil},
		{"people moment mentions in content", Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now.", People: ana},
			"<@123456789012>", "", []string{"123456789012"}},
		{"staff moment names nobody", Message{Moment: MomentWaiting, Subject: "ana asked", Text: "x", People: ana}, "", "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, content, e := embedOf(t, discordBody(c.m))
			if content != c.content {
				t.Errorf("content %q, want %q", content, c.content)
			}
			if e["title"] != c.m.Subject || e["description"] != c.m.Text {
				t.Errorf("embed %v", e)
			}
			if col, _ := e["color"].(float64); col <= 0 {
				t.Errorf("colour %v", e["color"])
			}
			th, _ := e["thumbnail"].(map[string]any)
			if got, _ := th["url"].(string); got != c.thumb {
				t.Errorf("thumbnail %v, want %q", e["thumbnail"], c.thumb)
			}
			if c.thumb == "" && e["thumbnail"] != nil {
				t.Errorf("thumbnail key %v", e["thumbnail"])
			}
			am, _ := p["allowed_mentions"].(map[string]any)
			users, _ := am["users"].([]any)
			if len(users) != len(c.mentioned) || p["username"] != "HolyArrs" {
				t.Errorf("allowed %v", p)
			}
		})
	}
}

// Embed limits hold: title 256, description 4096 letters.
func TestDiscordBodyLimits(t *testing.T) {
	m := Message{Moment: MomentImport, Subject: strings.Repeat("ä", 400), Text: strings.Repeat("ü", 5000)}
	_, _, e := embedOf(t, discordBody(m))
	if n := len([]rune(e["title"].(string))); n > 256 {
		t.Errorf("title %d", n)
	}
	if n := len([]rune(e["description"].(string))); n > 4096 {
		t.Errorf("description %d", n)
	}
}

// An import that replaced a file is titled Upgraded and shows the old and
// new quality, release and size; a plain import stays In the Library. The
// movie's stored public poster rides along.
func TestUpgradeVersusImport(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	id := movieWorld(t, s)
	setPoster(t, s, id, "https://image.tmdb.org/t/p/w342/arrival.jpg")
	for _, c := range []struct {
		name    string
		data    map[string]any
		subject string
		lines   []string
		color   int
	}{
		{"plain import", map[string]any{"import_id": 1}, "In the Library: Arrival (2016)", nil, colorImport},
		{"upgrade", map[string]any{"upgraded": true, "old_quality": "720p WEBDL", "new_quality": "2160p BLURAY HDR10",
			"old_release": "Arrival.2016.720p.WEB-DL-A", "new_release": "Arrival.2016.2160p.BluRay-B", "old_size": float64(2 << 30), "new_size": int64(40 << 30)},
			"Upgraded: Arrival (2016)", []string{"quality: 720p WEBDL → 2160p BLURAY HDR10", "release: Arrival.2016.720p.WEB-DL-A → Arrival.2016.2160p.BluRay-B", "size: 2.0 GB → 40.0 GB"}, colorUpgrade},
		{"upgrade with few facts", map[string]any{"upgraded": true, "new_quality": "1080p"}, "Upgraded: Arrival (2016)", []string{"quality: ? → 1080p"}, colorUpgrade},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := domain.Event{Kind: "import_done", TitleID: &id, Summary: "Import done: 1 placed", Data: c.data}
			m, ok := snd.MessageFor(ctx, e)
			if !ok || m.Subject != c.subject || m.Poster != "https://image.tmdb.org/t/p/w342/arrival.jpg" {
				t.Fatalf("message %+v", m)
			}
			for _, l := range c.lines {
				if !strings.Contains(m.Text, l) {
					t.Errorf("text %q lacks %q", m.Text, l)
				}
			}
			_, _, em := embedOf(t, discordBody(m))
			if int(em["color"].(float64)) != c.color {
				t.Errorf("colour %v", em["color"])
			}
		})
	}
}

// Episodes carry the series poster; with pack_alerts each every pack
// message is one embed with that poster and the Upgraded title.
func TestPackEmbedsCarryShowPoster(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid, slots := showWorld(t, s)
	setPoster(t, s, tid, "https://static.tvmaze.com/uploads/p.jpg")
	imp := domain.Event{Kind: "import_done", TitleID: &tid, SlotID: &slots[0], Summary: "Imported",
		Data: map[string]any{"upgraded": true, "placed_slot_ids": floats(slots[:2])}}
	ms := snd.MessagesFor(ctx, imp)
	if len(ms) != 1 || ms[0].Subject != "Upgraded: Copper Valley S01E01 to S01E02 (2 episodes)" || ms[0].Poster == "" {
		t.Fatalf("one %+v", ms)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackEach}, domain.System); err != nil {
		t.Fatal(err)
	}
	ms = snd.MessagesFor(ctx, imp)
	if len(ms) != 2 || ms[1].Subject != "Upgraded: Copper Valley S01E02" {
		t.Fatalf("each %+v", ms)
	}
	for _, m := range ms {
		_, _, e := embedOf(t, discordBody(m))
		if th, _ := e["thumbnail"].(map[string]any); th["url"] != "https://static.tvmaze.com/uploads/p.jpg" {
			t.Errorf("thumb %v", e)
		}
	}
}

// An album import shows the album cover from the metadata server; with no
// album id the artist photo; with no picture base nothing.
func TestAlbumEmbedArt(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	tid, slots := musicWorld(t, s, 2)
	imp := domain.Event{Kind: "import_done", TitleID: &tid, Summary: "Imported", Data: map[string]any{"placed_slot_ids": floats(slots)}}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}, Pictures: pictures}
	if m, _ := snd.MessageFor(ctx, imp); m.Poster != pictures+"/album/10000000-0000-0000-0000-000000000009/500.webp" {
		t.Fatalf("album art %q", m.Poster)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE parts SET source_ids = '{}'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if m, _ := snd.MessageFor(ctx, imp); m.Poster != pictures+"/artist/a0000000-0000-0000-0000-000000000009/500.webp" {
		t.Fatalf("artist art %q", m.Poster)
	}
	snd.Pictures = ""
	if m, _ := snd.MessageFor(ctx, imp); m.Poster != "" {
		t.Fatalf("no base %q", m.Poster)
	}
}

// Through the wire: the fake Discord gets one embed per message.
func TestDiscordSendsEmbed(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeDiscord(t)
	if _, err := Add(ctx, s, v, Input{Name: "D", Kind: "discord", URL: srv.URL + "/api/webhooks/1/abc", AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.Send(ctx, Message{Moment: MomentGrab, Subject: "Sent to download: Arrival", Text: "release: x"}); err != nil {
		t.Fatal(err)
	}
	g := got()
	if len(g) != 1 {
		t.Fatalf("posts %v", g)
	}
	es, _ := g[0]["embeds"].([]any)
	if len(es) != 1 || es[0].(map[string]any)["title"] != "Sent to download: Arrival" {
		t.Fatalf("post %v", g[0])
	}
	if _, has := g[0]["content"]; has {
		t.Fatalf("empty content was sent: %v", g[0])
	}
}

// An empty text leaves the description out (Discord refuses an empty one).
func TestDiscordBodyEmptyText(t *testing.T) {
	_, _, e := embedOf(t, discordBody(Message{Moment: MomentHealth, Subject: "Health: fixed"}))
	if _, has := e["description"]; has || e["title"] != "Health: fixed" {
		t.Fatalf("embed %v", e)
	}
}
