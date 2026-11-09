package accounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func arrKeyLastUsed(t *testing.T, f *fixture, side string) string {
	t.Helper()
	var v *string
	if err := f.s.Read().QueryRow("SELECT last_used_at FROM compat_keys WHERE side = ?", side).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func ownerReq(t *testing.T, f *fixture) (*http.Request, int64) {
	t.Helper()
	id, name, err := ownerIdentity(context.Background(), f.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: id, Name: name}))
	return r, id
}

func TestArrKeyTouchOncePerHour(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	ctx := context.Background()
	r, _ := ownerReq(t, f)
	k, err := f.a.MakeArrKey(r, "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	t0 := f.clock.now()
	who, ok := f.a.checkArrKey(ctx, "sonarr", k)
	if !ok || who.Via != "api_key" || !slices.Equal(who.Scopes, []string{webserver.ScopeCompat}) {
		t.Fatalf("check: %v %+v", ok, who)
	}
	if got := arrKeyLastUsed(t, f, "sonarr"); got != database.FormatTime(t0) {
		t.Fatalf("first touch %q", got)
	}
	f.clock.add(10 * time.Minute)
	if _, ok := f.a.checkArrKey(ctx, "sonarr", k); !ok {
		t.Fatal("refused")
	}
	if got := arrKeyLastUsed(t, f, "sonarr"); got != database.FormatTime(t0) {
		t.Fatalf("touched again within the hour: %q", got)
	}
	f.clock.add(110 * time.Minute)
	if _, ok := f.a.checkArrKey(ctx, "sonarr", k); !ok {
		t.Fatal("refused")
	}
	if got := arrKeyLastUsed(t, f, "sonarr"); got != database.FormatTime(t0.Add(2*time.Hour)) {
		t.Fatalf("not touched after two hours: %q", got)
	}
	if _, ok := f.a.checkArrKey(ctx, "radarr", k); ok {
		t.Fatal("key works on the other side")
	}
}

func TestArrKeysListShowsOwnerState(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	ctx := context.Background()
	r, _ := ownerReq(t, f)
	k, err := f.a.MakeArrKey(r, "radarr")
	if err != nil {
		t.Fatal(err)
	}
	ks, err := f.a.ArrKeys(ctx)
	if err != nil || len(ks) != 1 {
		t.Fatalf("list: %v %+v", err, ks)
	}
	v := ks[0]
	if v.Side != "radarr" || v.Owner != "luis" || v.Hint != "…"+k[len(k)-4:] || !v.OwnerOK {
		t.Fatalf("view %+v", v)
	}
	disableOwner(t, f.s)
	ks, err = f.a.ArrKeys(ctx)
	if err != nil || len(ks) != 1 || ks[0].OwnerOK {
		t.Fatalf("after disable: %v %+v", err, ks)
	}
	if _, ok := f.a.checkArrKey(ctx, "radarr", k); ok {
		t.Fatal("key of a disabled maker still works")
	}
}
