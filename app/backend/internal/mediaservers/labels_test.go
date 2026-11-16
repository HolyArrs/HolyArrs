package mediaservers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// plexLabelServer plays a Plex server with one movie (tmdb 603, key 50) and
// one show (tvdb 100, key 7) whose labels can be read and set the way Plex
// does: a PUT with label[n].tag.tag sets the whole list.
type plexLabelServer struct {
	*httptest.Server
	mu     sync.Mutex
	labels map[string][]string // ratingKey -> labels
	hidden map[string]bool     // ratingKey not scanned yet
	fourK  bool                // a second movie section holds the movie too (key 51)
	puts   []url.Values
}

func newPlexLabelServer(t *testing.T) *plexLabelServer {
	p := &plexLabelServer{labels: map[string][]string{"50": {"Favs"}}, hidden: map[string]bool{}}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		item := func(tag, rk, guid string) string {
			if p.hidden[rk] {
				return ""
			}
			return `<` + tag + ` ratingKey="` + rk + `" guid="plex://x/` + rk + `"><Guid id="` + guid + `"/></` + tag + `>`
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections":
			extra := ""
			if p.fourK {
				extra = `<Directory key="3" type="movie" title="Movies 4K"/>`
			}
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Movies"/><Directory key="2" type="show" title="TV"/>` + extra + `</MediaContainer>`))
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections/3/all":
			_, _ = w.Write([]byte(`<MediaContainer>` + item("Video", "51", "tmdb://603") + `</MediaContainer>`))
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections/1/all":
			_, _ = w.Write([]byte(`<MediaContainer>` + item("Video", "50", "tmdb://603") + `</MediaContainer>`))
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections/2/all":
			_, _ = w.Write([]byte(`<MediaContainer>` + item("Directory", "7", "tvdb://100") + `</MediaContainer>`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			rk := strings.TrimPrefix(r.URL.Path, "/library/metadata/")
			tag := "Video"
			if rk == "7" {
				tag = "Directory"
			}
			var b strings.Builder
			b.WriteString(`<MediaContainer><` + tag + ` ratingKey="` + rk + `">`)
			for _, lb := range p.labels[rk] {
				b.WriteString(`<Label id="1" tag="` + lb + `"/>`)
			}
			b.WriteString(`</` + tag + `></MediaContainer>`)
			_, _ = w.Write([]byte(b.String()))
		case r.Method == http.MethodPut && (r.URL.Path == "/library/sections/1/all" || r.URL.Path == "/library/sections/2/all" || r.URL.Path == "/library/sections/3/all"):
			q := r.URL.Query()
			p.puts = append(p.puts, q)
			var set []string
			for i := 0; ; i++ {
				v, ok := q["label["+string(rune('0'+i))+"].tag.tag"]
				if !ok {
					break
				}
				set = append(set, v[0])
			}
			p.labels[q.Get("id")] = set
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *plexLabelServer) state() (map[string][]string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[string][]string{}
	for k, v := range p.labels {
		out[k] = slices.Clone(v)
	}
	return out, len(p.puts)
}

// labelWorld is a Library with the movie and show, a Plex server, and an
// asked list that records the since it was asked with.
func labelWorld(t *testing.T) (*world, *plexLabelServer, *Linker, *[]string) {
	t.Helper()
	w := newWorld(t)
	movie, show := watchTitles(t, w)
	p := newPlexLabelServer(t)
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: p.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	var sinces []string
	l := &Linker{Store: w.s, Vault: w.v, Asked: func(_ context.Context, since string) ([]AskedTitle, error) {
		sinces = append(sinces, since)
		return []AskedTitle{{TitleID: movie, Askers: []string{"ana", "Ana", "bo"}}, {TitleID: show, Askers: []string{"ana"}}}, nil
	}}
	return w, p, l, &sinces
}

func labelsOn(t *testing.T, w *world, set LabelSettings) {
	t.Helper()
	if err := SaveLabelSettings(context.Background(), w.s, set, domain.System); err != nil {
		t.Fatal(err)
	}
}

func TestPlexLabelsOffCallsNothing(t *testing.T) {
	w, p, l, sinces := labelWorld(t)
	if got := ReadLabelSettings(context.Background(), w.s.Read()); got.Enabled || got.Label != "holyarrs" || got.Askers {
		t.Fatalf("defaults %+v", got)
	}
	if n, err := l.LabelPlex(context.Background(), true); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, puts := p.state(); puts != 0 || len(*sinces) != 0 {
		t.Fatalf("puts %d sinces %v", puts, *sinces)
	}
}

func TestPlexLabelsKeepOtherLabelsAndPutOnce(t *testing.T) {
	w, p, l, sinces := labelWorld(t)
	ctx := context.Background()
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "holyarrs", Askers: true})
	n, err := l.LabelPlex(ctx, false)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	got, puts := p.state()
	if !slices.Equal(got["50"], []string{"Favs", "holyarrs", "ana", "bo"}) || !slices.Equal(got["7"], []string{"holyarrs", "ana"}) || puts != 2 {
		t.Fatalf("labels %v puts %d", got, puts)
	}
	q := p.puts[0]
	if q.Get("type") != "1" || q.Get("id") != "50" || q.Get("label.locked") != "0" {
		t.Fatalf("put %v", q)
	}
	if (*sinces)[0] == "" {
		t.Fatal("the scheduled pass asked for every title, not those since labels were turned on")
	}

	// Someone removes "holyarrs" in Plex: it is not put back.
	p.mu.Lock()
	p.labels["50"] = []string{"Favs"}
	p.mu.Unlock()
	if n, err := l.LabelPlex(ctx, true); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if got, puts := p.state(); puts != 2 || !slices.Equal(got["50"], []string{"Favs"}) {
		t.Fatalf("labels %v puts %d", got, puts)
	}
	if (*sinces)[1] != "" {
		t.Fatalf("label existing asked since %q", (*sinces)[1])
	}
}

// A title in two sections (Movies and Movies 4K) gets the label in both.
func TestPlexLabelsEveryCopyOfATitle(t *testing.T) {
	w, p, l, _ := labelWorld(t)
	p.fourK = true
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "holyarrs"})
	if _, err := l.LabelPlex(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	got, _ := p.state()
	if !slices.Equal(got["50"], []string{"Favs", "holyarrs"}) || !slices.Equal(got["51"], []string{"holyarrs"}) {
		t.Fatalf("labels %v", got)
	}
}

func TestPlexLabelsRetryUntilPlexHasIt(t *testing.T) {
	w, p, l, _ := labelWorld(t)
	ctx := context.Background()
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "  From   HolyArrs "})
	p.mu.Lock()
	p.hidden["7"] = true
	p.labels["50"] = []string{"from holyarrs"} // already there in another case
	p.mu.Unlock()
	if n, err := l.LabelPlex(ctx, false); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, puts := p.state(); puts != 0 {
		t.Fatalf("puts %d", puts)
	}
	p.mu.Lock()
	p.hidden["7"] = false
	p.mu.Unlock()
	if n, err := l.LabelPlex(ctx, false); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if got, _ := p.state(); !slices.Equal(got["7"], []string{"From HolyArrs"}) {
		t.Fatalf("labels %v", got)
	}
}

func TestPlexLabelsSkipOtherServers(t *testing.T) {
	w, p, l, _ := labelWorld(t)
	ctx := context.Background()
	jelly := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		t.Errorf("Jellyfin was called: %s", r.URL.Path)
	}))
	t.Cleanup(jelly.Close)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Jelly", Kind: "jellyfin", URL: jelly.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "holyarrs"})
	if n, err := l.LabelPlex(ctx, true); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if _, puts := p.state(); puts != 2 {
		t.Fatalf("puts %d", puts)
	}
}

func TestPlexLabelSettingsSinceStartsWhenTurnedOn(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "a"})
	first := ReadLabelSettings(ctx, w.s.Read()).Since
	labelsOn(t, w, LabelSettings{Enabled: true, Label: "b", Askers: true})
	if got := ReadLabelSettings(ctx, w.s.Read()); got.Since != first || got.Label != "b" || !got.Askers || first == "" {
		t.Fatalf("since %q then %+v", first, got)
	}
}

func TestPlexLabelRoutes(t *testing.T) {
	w := newWorld(t)
	l := &Linker{Store: w.s, Vault: w.v}
	mux := http.NewServeMux()
	(&Handlers{Store: w.s, Vault: w.v, Linker: l}).Routes(mux)
	send := func(method, path, body string, want int) string {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s %s: %d %s", method, path, body, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	if b := send("GET", "/api/v1/settings/plex-labels", "", 200); b != `{"enabled":false,"label":"holyarrs","askers":false}`+"\n" {
		t.Fatalf("default %s", b)
	}
	send("POST", "/api/v1/settings/plex-labels/run", "", 409)
	send("PUT", "/api/v1/settings/plex-labels", `{"label":"   "}`, 400)
	send("PUT", "/api/v1/settings/plex-labels", `{"label":"`+strings.Repeat("x", 65)+`"}`, 400)
	send("PUT", "/api/v1/settings/plex-labels", `nope`, 400)
	if b := send("PUT", "/api/v1/settings/plex-labels", `{"enabled":true,"label":" Asked ","askers":true}`, 200); b != `{"enabled":true,"label":"Asked","askers":true}`+"\n" {
		t.Fatalf("saved %s", b)
	}
	send("POST", "/api/v1/settings/plex-labels/run", "", 202)
	l.Wait()
}
