package metadata

import (
	"context"
	"net/http"
	"testing"
)

type statusAnswer int

func (s statusAnswer) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: int(s), Body: http.NoBody, Header: http.Header{}}, nil
}

// A credential the server no longer knows is dropped once, and the next
// call registers a new one. Other answers, and other credentials, change
// nothing.
func TestUnknownKeyRegistersAgain(t *testing.T) {
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", "")
	v := testVault(t)
	ctx := context.Background()
	refused := dropUnknownKey(v)
	n := 0
	access := installationAccess(v, func(context.Context) (string, error) {
		n++
		return []string{"first-credential-000000", "second-credential-00000", "third-credential-000000"}[n-1], nil
	})
	call := func(status int, tok string) {
		req, _ := http.NewRequest(http.MethodGet, "https://example.org/v1/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		if _, err := (keyCheck{next: statusAnswer(status), refused: refused}).RoundTrip(req); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := access(ctx)
	call(200, first)
	call(429, first)
	call(401, "someone-elses-credential")
	if tok, _ := access(ctx); tok != first || n != 1 {
		t.Fatalf("dropped without a refusal: %q after %d registrations", tok, n)
	}
	call(401, first)
	second, _ := access(ctx)
	if second == first || n != 2 {
		t.Fatalf("no new credential after a refusal: %q after %d registrations", second, n)
	}
	// Refused again straight away: it waits, so a broken server is not flooded.
	call(401, second)
	if tok, _ := access(ctx); tok != second || n != 2 {
		t.Fatalf("registered again without waiting: %q after %d", tok, n)
	}
}
