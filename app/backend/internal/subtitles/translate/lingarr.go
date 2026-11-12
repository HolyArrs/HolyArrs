package translate

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Lingarr talks to a Lingarr server, which runs its own translation service
// (LibreTranslate, a local model or a cloud one). URL is the server's
// address and Key its optional API key.
type Lingarr struct {
	URL, Key string
	HTTP     *http.Client
	Timeout  time.Duration // per call; ten minutes when 0, since a sleeping model may need to wake
}

// tag writes a language code the way Lingarr expects it, with the region in
// capitals: pt-br becomes pt-BR.
func tag(code string) string {
	if base, region, ok := strings.Cut(code, "-"); ok {
		return base + "-" + strings.ToUpper(region)
	}
	return code
}

// Translate posts one chunk to the content route and reads the lines back by
// position.
func (l *Lingarr) Translate(ctx context.Context, b Batch) ([]Line, error) {
	type wire struct {
		Position int    `json:"position"`
		Line     string `json:"line"`
	}
	lines := make([]wire, len(b.Lines))
	for i, x := range b.Lines {
		lines[i] = wire{x.N, x.Text}
	}
	media := "Movie"
	if b.Kind == "episode" {
		media = "Episode"
	}
	body, _ := json.Marshal(map[string]any{
		"arrMediaId": b.MediaID, "title": b.Title, "sourceLanguage": tag(b.From), "targetLanguage": tag(b.To),
		"mediaType": media, "lines": lines,
	})
	hdr := map[string]string{}
	if l.Key != "" {
		hdr["X-Api-Key"] = l.Key
	}
	timeout := l.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	out, err := send(ctx, l.HTTP, http.MethodPost, endpoint(l.URL, "/api/translate/content"), hdr, body, timeout, l.Key)
	if err != nil {
		return nil, err
	}
	got := jsonLines(out)
	if len(got) == 0 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the answer held no translated lines"}
	}
	return got, nil
}
