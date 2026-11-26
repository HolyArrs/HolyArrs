package migrate

import (
	"context"
	"crypto/subtle"

	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// sameAppReads finds the kept reads of src's app at other addresses that
// no import used yet. An app's API key is its identity: the key kept for
// an older read's address matching src's key means the same app reached
// another way (sonarr.example.com:8989 and 10.0.0.5:8989). The keys stay
// on the server; only the read numbers come back.
func (h *Handlers) sameAppReads(ctx context.Context, src Source, address string) []Saved {
	if h.Secrets == nil || src.Key == "" || src.URL == "" {
		return nil
	}
	list, err := List(ctx, h.Store)
	if err != nil {
		return nil
	}
	var out []Saved
	for _, s := range list {
		if s.Kind != src.Kind || s.Address == "" || s.Address == address || snapshotUsed(ctx, h.Store.Read(), s.ID) {
			continue
		}
		c, _, has, err := secretsfile.ReadMoveCredentialAt(ctx, h.Store, h.Secrets, string(s.Kind), s.Address)
		if err != nil || !has || c.APIKey == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(c.APIKey), []byte(src.Key)) == 1 {
			out = append(out, s)
		}
	}
	return out
}

// dropSameApp forgets the older reads of the same app (and the keys kept
// for their addresses), so one app counts once; it returns their numbers.
func (h *Handlers) dropSameApp(ctx context.Context, old []Saved) []int64 {
	ids := []int64{}
	for _, s := range old {
		if err := Delete(ctx, h.Store, s.ID); err != nil {
			continue
		}
		_, _ = secretsfile.ForgetMoveCredentialAt(ctx, h.Store, h.Secrets, string(s.Kind), s.Address)
		ids = append(ids, s.ID)
	}
	return ids
}
