package mediaservers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestPlaybackPosterRequestsSizedImage(t *testing.T) {
	w := newWorld(t)
	poster, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFklEQVR4nGNk+M/AwMDAxMDAwMDAAAAZ5wEEtUNqgwAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	const thumb = "/library/metadata/18/thumb/1"
	plex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/photo/:/transcode" || query.Get("url") != thumb ||
			query.Get("width") != "360" || query.Get("height") != "540" || r.Header.Get("X-Plex-Token") != tok {
			http.Error(w, "request must ask for the sized cover", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(poster)
	}))
	defer plex.Close()
	on := true
	m, err := Add(t.Context(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: plex.URL, Token: tok, MonitorPlayback: &on}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	state, err := playbackStateFor(t.Context(), w.s.Read(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := playbackKey{Session: "preview", Account: "1", Item: "18"}
	if _, err := savePlaybackSnapshot(t.Context(), w.s, m.ID, state.generation,
		map[playbackKey]playbackSample{key: {thumb: thumb}}, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	view := readPlaybackForTest(t, w, m, playbackFilter{}, time.Now())
	h := Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v}}
	routes := http.NewServeMux()
	h.Routes(routes)

	ctx := webserver.ContextWith(t.Context(), webserver.Caller{Role: webserver.RoleOwner, Via: "session"})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/api/v1/media-servers/%d/playback/%d/poster", m.ID, view.Active[0].ID), nil)
	answer := httptest.NewRecorder()
	routes.ServeHTTP(answer, req)
	if answer.Code != http.StatusOK || answer.Header().Get("Content-Type") != "image/png" || answer.Body.Len() != len(poster) {
		t.Fatalf("poster response = %d %s (%d bytes)", answer.Code, answer.Header().Get("Content-Type"), answer.Body.Len())
	}
}
