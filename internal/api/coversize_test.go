package api

import (
	"net/http/httptest"
	"testing"
)

func TestCoverSize(t *testing.T) {
	for q, want := range map[string]string{"": "250", "?size=500": "500", "?size=1200": "1200", "?size=999": "250"} {
		if got := coverSize(httptest.NewRequest("GET", "/x"+q, nil)); got != want {
			t.Errorf("coverSize(%q) = %s, want %s", q, got, want)
		}
	}
}
