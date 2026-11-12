package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate/faketranslator"
)

func call(m *http.ServeMux, method, path, body string) (int, string) {
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec.Code, rec.Body.String()
}

// route: GET /api/v1/settings/subtitles/translator
// route: PUT /api/v1/settings/subtitles/translator
// route: POST /api/v1/settings/subtitles/translator/test
// route: POST /api/v1/files/{id}/subtitles/translate
func TestRoutes_TranslatorAndTranslate(t *testing.T) {
	f := newFix(t)
	labMovie(t, f)
	ft := &faketranslator.OpenAI{Models: []string{"lab-model", "other"}}
	s, q := withTranslator(t, f, ft)
	m := http.NewServeMux()
	NewHandlers(f.st, s.Vault, s).Routes(m)

	code, body := call(m, "GET", "/api/v1/settings/subtitles/translator", "")
	if code != 200 || strings.Contains(body, fakeAIKey) || !strings.Contains(body, `"has_key":true`) {
		t.Fatalf("get %d %s", code, body)
	}
	// Leaving fields out keeps them; the key stays put and is never sent back.
	code, body = call(m, "PUT", "/api/v1/settings/subtitles/translator", `{"built_in_auto": true}`)
	if code != 200 || strings.Contains(body, fakeAIKey) || !strings.Contains(body, `"built_in_auto":true`) || !strings.Contains(body, `"model":"lab-model"`) {
		t.Fatalf("put %d %s", code, body)
	}
	// Moving the address to another host without the key again is refused.
	code, body = call(m, "PUT", "/api/v1/settings/subtitles/translator", `{"url": "http://elsewhere.example.test:11434"}`)
	if code != http.StatusConflict {
		t.Fatalf("moved host %d %s", code, body)
	}
	code, body = call(m, "PUT", "/api/v1/settings/subtitles/translator", `{"kind": "nope"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad kind %d %s", code, body)
	}
	code, body = call(m, "POST", "/api/v1/settings/subtitles/translator/test", `{}`)
	var tr TranslatorTest
	_ = json.Unmarshal([]byte(body), &tr)
	if code != 200 || !tr.OK || len(tr.Models) != 2 || strings.Contains(body, fakeAIKey) {
		t.Fatalf("test %d %s", code, body)
	}

	code, body = call(m, "POST", "/api/v1/files/"+itoa(f.movieFile)+"/subtitles/translate", `{"language": "pt-BR", "from": "en"}`)
	if code != http.StatusAccepted || len(*q) != 1 || (*q)[0].kind != KindTranslate || (*q)[0].params["by_hand"] != true {
		t.Fatalf("translate %d %s %+v", code, body, *q)
	}
	if code, _ := call(m, "POST", "/api/v1/files/"+itoa(f.movieFile)+"/subtitles/translate", `{"language": "xx"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown language %d", code)
	}
	if code, _ := call(m, "POST", "/api/v1/files/99999/subtitles/translate", `{"language": "pt-br"}`); code != http.StatusNotFound {
		t.Fatalf("no such file %d", code)
	}
	// A key cleared and the translator off: asking says so.
	if code, body := call(m, "PUT", "/api/v1/settings/subtitles/translator", `{"kind": "", "url": "", "model": "", "clear_key": true}`); code != 200 || !strings.Contains(body, `"has_key":false`) {
		t.Fatalf("off %d %s", code, body)
	}
	if code, body := call(m, "POST", "/api/v1/files/"+itoa(f.movieFile)+"/subtitles/translate", `{"language": "pt-br"}`); code != http.StatusConflict || !strings.Contains(body, "no translator") {
		t.Fatalf("off translate %d %s", code, body)
	}
}

// A picture track is refused by name before anything is queued.
func TestRoutes_TranslatePictureTrackRefused(t *testing.T) {
	f := newFix(t)
	labMovie(t, f)
	s, q := withTranslator(t, f, &faketranslator.OpenAI{})
	ctx := context.Background()
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	tracks, _ := container.Probe(f.abs(movieRel))
	track := tracks[len(tracks)-1].Index
	// The lab file has text tracks only; record one as PGS to stand for a
	// picture track.
	err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE subtitles SET codec = 'S_HDMV/PGS' WHERE file_id = ? AND place = 'built_in' AND track_index = ?", f.movieFile, track)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	NewHandlers(f.st, s.Vault, s).Routes(m)
	code, body := call(m, "POST", "/api/v1/files/"+itoa(f.movieFile)+"/subtitles/translate", `{"language": "pt-br", "source": "built_in", "track": `+itoa(int64(track))+`}`)
	if code != http.StatusConflict || !strings.Contains(body, "picture") || len(*q) != 0 {
		t.Fatalf("%d %s %+v", code, body, *q)
	}
}
