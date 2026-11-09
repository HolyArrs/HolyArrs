package music

import (
	"net/http/httptest"
	"testing"
)

func TestCoverSize(t *testing.T) {
	t.Parallel()
	for q, want := range map[string]string{"": "500", "?size=250": "250", "?size=500": "500", "?size=1200": "1200", "?size=999": "500"} {
		if got := CoverSize(httptest.NewRequest("GET", "/x"+q, nil)); got != want {
			t.Errorf("CoverSize(%q) = %s, want %s", q, got, want)
		}
	}
}
