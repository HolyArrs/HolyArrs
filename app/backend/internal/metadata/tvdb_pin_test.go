package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTVDBLogsInWithKeyAndPIN(t *testing.T) {
	const pin = "fixture-pin"
	var sent map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/login" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		sent = nil
		_ = json.NewDecoder(r.Body).Decode(&sent)
		if sent["apikey"] != fakeTVDBKey || sent["pin"] != pin {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"` + "tok" + `en":"fixture-session"}}`))
	}))
	defer srv.Close()
	cur := pin
	p := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return fakeTVDBKey, nil },
		PIN: func(context.Context) (string, error) { return cur, nil }, Gap: time.Millisecond}
	ctx := context.Background()
	if _, err := p.session(ctx, false); err != nil {
		t.Fatalf("key and PIN: %v", err)
	}
	cur = "wrong"
	_, err := p.session(ctx, false)
	if !errors.Is(err, ErrTVDBRefused) || !errors.Is(err, ErrNoTVDBKey) || err.Error() != "Key or PIN refused" {
		t.Fatalf("a changed, wrong PIN logs in again and is refused: %v", err)
	}
	if sent["pin"] != "wrong" {
		t.Fatalf("PIN not sent: %v", sent)
	}
}
