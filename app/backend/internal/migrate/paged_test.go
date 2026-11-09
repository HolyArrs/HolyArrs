package migrate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A paged list stops once its pages add up to a few answers' worth, even
// when each page fits and the total it claims never comes.
func TestPagedReadStopsOnTotalSize(t *testing.T) {
	pad := strings.Repeat("x", 3000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"totalRecords":500000,"records":[{"id":1,"pad":%q}]}`, pad)
	}))
	defer srv.Close()
	c, err := newAPIClient(Source{Kind: Radarr, URL: srv.URL, AllowLocal: true, Limits: Limits{MaxAnswer: 4096}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (arrReader{kind: Radarr}).pages(context.Background(), c, "/exclusions"); !errors.Is(err, ErrTooBig) {
		t.Fatalf("want ErrTooBig, got %v", err)
	}
}
