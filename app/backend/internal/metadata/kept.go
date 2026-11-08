package metadata

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// keptOnDisk is how long a stored outside answer stays in SQLite; it is
// refreshed long before (in the background, on view).
const keptOnDisk = 30 * 24 * time.Hour

// KeptPerPrefix caps the rows each kind of kept answer ("extras|",
// "wiki|", "details|") holds in lookup_cache; the oldest go first.
var KeptPerPrefix = 20000

// Kept is an outside answer read back from lookup_cache with when it was
// fetched.
type Kept[T any] struct {
	V  T         `json:"v"`
	At time.Time `json:"at"`
}

// KeepOnDisk stores v under key in lookup_cache with when it was fetched.
func KeepOnDisk[T any](ctx context.Context, store *database.Store, key string, v T) {
	if store == nil {
		return
	}
	now := time.Now()
	if b, err := json.Marshal(Kept[T]{V: v, At: now}); err == nil {
		ctx := context.WithoutCancel(ctx)
		_ = LookupCache{Store: store}.Put(ctx, key, b, now, now.Add(keptOnDisk))
		capKept(ctx, store, key)
	}
}

// LoadKept reads an answer KeepOnDisk stored.
func LoadKept[T any](ctx context.Context, store *database.Store, key string) (Kept[T], bool) {
	var k Kept[T]
	if store == nil || store.Read() == nil {
		return k, false
	}
	b, ok, err := LookupCache{Store: store}.Get(ctx, key, time.Now())
	if err != nil || !ok || json.Unmarshal(b, &k) != nil {
		return Kept[T]{}, false
	}
	return k, true
}

var refreshing sync.Map

// RefreshOnce runs fn in the background unless a refresh for key is
// already running; the visitor never waits for it.
func RefreshOnce(key string, fn func(ctx context.Context)) {
	if _, busy := refreshing.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer refreshing.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

// DigitsOnly reports whether s is a TMDB-style id: digits, no leading zero.
func DigitsOnly(s string) bool {
	if s == "" || len(s) > 12 || s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// capKept drops the oldest rows of key's prefix past KeptPerPrefix.
func capKept(ctx context.Context, store *database.Store, key string) {
	prefix, _, ok := strings.Cut(key, "|")
	if !ok {
		return
	}
	_ = LookupCache{Store: store}.TrimPrefix(ctx, prefix+"|", KeptPerPrefix)
}
