package downloads

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// The setup wizard saves qBittorrent through the same route as Settings:
// an API key alone, a user name and password, and a switch either way all
// save, and an empty user name means the key is used.
func TestQbitSignInWayBothWays(t *testing.T) {
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	str := func(x string) *string { return &x }
	fake := func(x string) string { return "lab-" + x }
	d, err := Add(ctx, s, v, Input{Name: "q", Adapter: "qbittorrent", URL: "http://10.0.0.9:8080", Secret: fake("k1")}, domain.System)
	if err != nil || d.Username != "" || d.SecretRef == nil {
		t.Fatalf("add with a key: %+v %v", d, err)
	}
	if d, err = Change(ctx, s, v, d.ID, Input{Name: "q", Username: str("admin"), Secret: fake("p1")}, domain.System); err != nil || d.Username != "admin" {
		t.Fatalf("to user and password: %+v %v", d, err)
	}
	// Back to a key without typing it: refused, so a password is never used as a key.
	if _, err = Change(ctx, s, v, d.ID, Input{Name: "q", Username: str("")}, domain.System); !errors.Is(err, ErrQbitKey) {
		t.Fatalf("to key without a key: %v", err)
	}
	if d, err = Change(ctx, s, v, d.ID, Input{Name: "q", Username: str(""), Secret: fake("k2")}, domain.System); err != nil || d.Username != "" {
		t.Fatalf("back to a key: %+v %v", d, err)
	}
	if _, err = Add(ctx, s, v, Input{Name: "q2", Adapter: "qbittorrent", URL: "http://10.0.0.9:8081"}, domain.System); !errors.Is(err, ErrQbitKey) {
		t.Fatalf("add with nothing: %v", err)
	}
}
