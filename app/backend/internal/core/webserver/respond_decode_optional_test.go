package webserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeOptional(t *testing.T) {
	for _, c := range []struct {
		body string
		ok   bool
		want []string
	}{
		{"", true, nil},
		{`{"languages": ["hr"]}`, true, []string{"hr"}},
		{`{}`, true, nil},
		{`{"languages": "hr"}`, false, nil},
		{`{`, false, nil},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
		var in struct {
			Languages []string `json:"languages"`
		}
		if got := DecodeOptional(w, r, &in, "help"); got != c.ok {
			t.Fatalf("%q: ok %v, want %v", c.body, got, c.ok)
		}
		if !c.ok && w.Code != http.StatusBadRequest {
			t.Fatalf("%q: status %d", c.body, w.Code)
		}
		if c.ok && strings.Join(in.Languages, ",") != strings.Join(c.want, ",") {
			t.Fatalf("%q: %v", c.body, in.Languages)
		}
	}
}
