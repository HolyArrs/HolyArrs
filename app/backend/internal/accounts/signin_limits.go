package accounts

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

const (
	knownPerName = 20   // places kept for one name
	knownNames   = 2000 // names kept at all
)

// maxTracked bounds the failure and known-place maps.
const maxTracked = 10000

// fullExtra caps the real-account records noteFail adds once the table is
// full, a hard ceiling in case names that look real are made up.
const fullExtra = 1000

// accountKeep is how long a real account's count is kept after its last
// failure, so a quiet spell between guessing rounds does not hand back free
// guesses. Its wait still ends after MaxBackoff; only the level is kept.
const accountKeep = 24 * time.Hour

// accountPrefixes are fail keys that name a user id, so their number is
// bounded by the users.
var accountPrefixes = []string{"2fa:", "reauth:", "pw:", "code:", "sso-proof:", "sso-test:"}

// place is where a sign-in came from, for known places.
type place struct {
	ipKey, userKey string
	sure           bool
}

// knownChange is one noteKnown for saveKnown: the place to store, and the
// places it pushed out (an empty ip key: every place of that name).
type knownChange struct {
	put  place
	at   time.Time
	gone [][2]string
}

// failMark is how one key stood before noteFail, so undoFail can put back
// its time as well as its count.
type failMark struct {
	key  string
	had  bool
	last time.Time
}

// ClientKey names the caller for rate limits: the peer address, or the
// client a trusted proxy forwards, by the shared rule in clientAddr.
// Forwarding headers from any other peer are ignored.
func (a *Auth) ClientKey(r *http.Request) string {
	k, _ := a.clientKeySure(r)
	return k
}

// clientKey is the key every sign-in limit uses: the proxy-aware ClientKey,
// so behind a trusted proxy each visitor gets their own budget.
func (a *Auth) clientKey(r *http.Request) string { return a.ClientKey(r) }

// clientKeySure is ClientKey plus whether the address is sure enough to stand
// for a known place (false when the proxy headers disagree).
func (a *Auth) clientKeySure(r *http.Request) (string, bool) {
	addr, _, unsure := a.clientAddrUnsure(r)
	return addrKey(addr, unsure), !unsure
}

// addrKey is the limit key of a client address. When the proxy headers
// disagree, the X-Real-IP address goes under its own "xri:" namespace: the
// visitor may have written it, so it must never read or charge the "ip:"
// count of a real visitor at that address.
func addrKey(addr string, unsure bool) string {
	if unsure {
		return "xri:" + limitAddr(addr)
	}
	return "ip:" + limitAddr(addr)
}

// clientKeys is clientKeySure plus altKey: when the proxy headers disagree,
// the key of the other header's address ("" otherwise). A failure counts under
// both and the larger wait stands. altKey has its own "xff:" namespace, read
// and charged only by requests whose headers disagree: a visitor can write any
// address in the header their proxy does not set, so that address's "ip:"
// count (a real visitor's) must never be touched by it.
func (a *Auth) clientKeys(r *http.Request) (ipKey, altKey string, sure bool) {
	addr, other, _, unsure := a.clientAddrBoth(r)
	if other != "" {
		altKey = "xff:" + limitAddr(other)
	}
	return addrKey(addr, unsure), altKey, !unsure
}

// signInKeys is clientKeys for password and code checks. Behind a private
// proxy that is not trusted every visitor shares the proxy's address, so
// there the address key is dropped (ipKey "") and only the account's own
// wait counts: one person's typos cannot lock everyone out. The Settings
// card already tells the owner to trust the proxy.
func (a *Auth) signInKeys(r *http.Request) (ipKey, altKey string, sure bool) {
	ipKey, altKey, sure = a.clientKeys(r)
	if a.sharedProxy(r) {
		return "", "", false
	}
	return ipKey, altKey, sure
}

// sharedProxy says whether the peer is the untrusted loopback or private
// proxy the warning saw, and it sent forwarding headers. The headers alone
// prove nothing, since any device can write them: a home device forging
// X-Forwarded-For keeps its own per-address limit.
func (a *Auth) sharedProxy(r *http.Request) bool {
	if !anyForwardHeader(r) {
		return false
	}
	ip := parseHostIP(r.RemoteAddr)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate()) && !a.trustedProxy(ip) && a.seenProxy(ip)
}

// altWait is the address wait for altKey, if there is one; the caller holds a.mu.
func (a *Auth) altWait(altKey string) time.Duration {
	if altKey == "" {
		return 0
	}
	return a.waitFor(altKey, freeTries)
}

// limitAddr is the address as limits count it: IPv4 as is (an IPv4-mapped
// IPv6 address as its IPv4 form), IPv6 as its /64, since one network owns a
// whole /64 and could otherwise bring unlimited fresh addresses. The
// local-network skip reads the exact address, not this.
func limitAddr(addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return addr
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// eventAddr is the exact caller address, for the Events log.
func (a *Auth) eventAddr(r *http.Request) string {
	addr, _ := a.clientAddr(r)
	if ip := net.ParseIP(addr); ip != nil {
		return ip.String()
	}
	return addr
}

// waitFor returns how long the key must still wait, or 0.
func (a *Auth) waitFor(key string, free int) time.Duration {
	f := a.fails[key]
	if f == nil || f.n < free {
		return 0
	}
	d := time.Second << min(f.n-free, 20)
	if d > MaxBackoff {
		d = MaxBackoff
	}
	if left := f.last.Add(d).Sub(a.now()); left > 0 {
		return left
	}
	return 0
}

// signInWait is how long a password sign-in must wait; the caller holds a.mu.
// The per-address wait always counts. The per-name wait counts for every
// address except one that signed in as that name in the last 30 days, so
// strangers spreading guesses over many addresses stay slowed, yet cannot
// keep the owner out of a place they used before (home, their phone's line).
func (a *Auth) signInWait(ipKey, userKey string, sure bool) time.Duration {
	wait := a.waitFor(ipKey, freeTries)
	if a.knownPlace(ipKey, userKey, sure) {
		return wait
	}
	return max(wait, a.waitFor(userKey, freeUserTries))
}

// knownPlace says whether ipKey signed in as userKey in the last 30 days.
func (a *Auth) knownPlace(ipKey, userKey string, sure bool) bool {
	if !sure {
		return false
	}
	t, ok := a.known[userKey][ipKey]
	return ok && a.now().Sub(t) < knownPlaceFor
}

// noteKnown remembers a finished sign-in for signInWait; the caller holds
// a.mu and passes the result to saveKnown once a.mu is unlocked, so the place
// still counts after a restart. Places are kept per name, so other names and
// addresses can never push a name's places out; only that name's own least
// recent place goes. An address that is not sure is never recorded.
func (a *Auth) noteKnown(p place) *knownChange {
	if !p.sure || p.userKey == "" {
		return nil
	}
	ch := &knownChange{put: p, at: a.now()}
	ch.gone = a.rememberKnown(p.userKey, p.ipKey, ch.at)
	return ch
}

// rememberKnown records userKey signing in from ipKey at the given time and
// returns the places pushed out to stay within the limits; the caller holds
// a.mu.
func (a *Auth) rememberKnown(userKey, ipKey string, at time.Time) (gone [][2]string) {
	if a.known == nil {
		a.known = map[string]map[string]time.Time{}
	}
	set := a.known[userKey]
	if set == nil {
		if len(a.known) >= knownNames {
			var oldest string
			var oldestAt time.Time
			for n, s := range a.known {
				var last time.Time
				for _, t := range s {
					if t.After(last) {
						last = t
					}
				}
				if oldest == "" || last.Before(oldestAt) {
					oldest, oldestAt = n, last
				}
			}
			delete(a.known, oldest)
			gone = append(gone, [2]string{oldest, ""})
		}
		set = map[string]time.Time{}
		a.known[userKey] = set
	}
	if _, ok := set[ipKey]; !ok && len(set) >= knownPerName {
		var oldest string
		var oldestAt time.Time
		for k, t := range set {
			if oldest == "" || t.Before(oldestAt) {
				oldest, oldestAt = k, t
			}
		}
		delete(set, oldest)
		gone = append(gone, [2]string{userKey, oldest})
	}
	set[ipKey] = at
	return gone
}

// saveKnown stores a noteKnown change, and drops that name's places older
// than knownPlaceFor. Best effort: a failed write is logged and never fails
// the sign-in; the place still counts until the next restart.
func (a *Auth) saveKnown(ch *knownChange) {
	if ch == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		for _, g := range ch.gone {
			if err := deleteKnownPlace(ctx, tx, g[0], g[1]); err != nil {
				return err
			}
		}
		if err := deleteOldKnownPlaces(ctx, tx, ch.put.userKey, database.FormatTime(ch.at.Add(-knownPlaceFor))); err != nil {
			return err
		}
		return putKnownPlace(ctx, tx, ch.put.userKey, ch.put.ipKey, database.FormatTime(ch.at))
	})
	if err != nil {
		log.Printf("auth: could not save a known sign-in place: %v", err)
	}
}

// loadKnown reads the known places of the last 30 days at start, oldest
// first, under the same limits as noteKnown.
func (a *Auth) loadKnown(ctx context.Context) {
	rows, err := knownPlacesSince(ctx, a.s.Read(), database.FormatTime(a.now().Add(-knownPlaceFor)))
	if err != nil {
		log.Printf("auth: could not read known sign-in places: %v", err)
		return
	}
	defer rows.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	for rows.Next() {
		var u, ip, at string
		if rows.Scan(&u, &ip, &at) == nil {
			if t := parseTime(at); !t.IsZero() {
				a.rememberKnown(u, ip, t)
			}
		}
	}
}

func (a *Auth) noteFail(keys ...string) {
	now := a.now()
	if len(a.fails) >= maxTracked && (len(a.fails) >= a.pruneAt || now.Sub(a.lastPruneAt) > MaxBackoff) {
		a.pruneFails(now)
		// Scan again only after the map has grown a little, so a flood that
		// leaves nothing to free does not cost a full scan per attempt.
		a.pruneAt = len(a.fails) + maxTracked/10
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		f := a.fails[k]
		if f == nil {
			// A name record is only added while the map has room for it, so
			// a flood of made-up names never pushes out a record that still
			// holds a wait. The address key still counts. Names use at most
			// three quarters of the room, leaving the rest for addresses.
			limit := 4 * maxTracked
			if protectedKey(k) {
				limit = 3 * maxTracked
			}
			if len(a.fails) >= limit && protectedKey(k) && now.Sub(a.lastPruneAt) >= time.Minute {
				a.pruneFails(now) // free expired records before refusing
			}
			if len(a.fails) >= limit {
				// A table full of names: made-up names get no record (no account
				// to protect); a real account still gets one, so a flood cannot
				// strip its wait. Real names are few (one per user), so this
				// stays bounded. Trade-off: only while a flood has filled the
				// table can an attacker tell real names from made-up ones, by
				// whether a name wait appears; fine for a self-hosted app with a
				// few users.
				// The ceiling sits above the address share, so address records
				// from the flood cannot use up the room real names get.
				if !protectedKey(k) || !a.isRealName(k, now) || len(a.fails) >= 4*maxTracked+fullExtra {
					if protectedKey(k) {
						a.noteSkipped(now)
					}
					continue
				}
			}
			f = &failure{}
			a.fails[k] = f
		}
		// An address count whose wait ended long ago starts over, so one
		// typo now and then at home never builds up to a lockout (pruning
		// does the same, but only once the table is big). A real account
		// keeps its count for accountKeep.
		if idle := now.Sub(f.last); f.n > 0 && idle > MaxBackoff && (idle > accountKeep || !a.accountKey(k, now)) {
			f.n = 0
		}
		f.n++
		f.last = now
	}
}

// noteSkipped logs once per window that made-up names get no record.
func (a *Auth) noteSkipped(now time.Time) {
	if now.Sub(a.satLogged) > MaxBackoff {
		a.satLogged = now
		log.Printf("auth: sign-in limits are full; names with no account are no longer tracked until the flood stops")
	}
}

// isRealName says whether a name:/2fa: key is for an account (a second-step
// key only exists for a ticket, so always). The user names are read once a
// minute, and only while the table is full; the caller holds a.mu.
func (a *Auth) isRealName(k string, now time.Time) bool {
	rest, ok := strings.CutPrefix(k, "name:")
	if !ok {
		return true
	}
	if a.realNames == nil || now.Sub(a.realNamesAt) >= time.Minute {
		names := map[string]bool{}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// A Jellyfin or Emby account linked to a member under another name
		// (taken, or renamed since) counts as real too, as "<kind>:<name>".
		if rows, err := realNameRows(ctx, a.s.Read()); err == nil {
			for rows.Next() {
				var n string
				if rows.Scan(&n) == nil {
					names[strings.ToLower(n)] = true
				}
			}
			_ = rows.Close()
		}
		a.realNames, a.realNamesAt = names, now
	}
	if a.realNames[rest] {
		return true
	}
	// name:<kind>:<name> for a Jellyfin or Emby sign-in; only those kinds,
	// so a typed name like "x1:ana" is not taken for the account ana.
	kind, after, ok := strings.Cut(rest, ":")
	return ok && (kind == ProviderJellyfin || kind == ProviderEmby) && a.realNames[after]
}

// protectedKey says whether a fail key is a per-name one (sign-in name or
// second-step ticket), which is kept for as long as its wait runs.
func protectedKey(k string) bool {
	return strings.HasPrefix(k, "name:") || strings.HasPrefix(k, "2fa:")
}

// accountKey says whether a fail record belongs to a real account; the caller
// holds a.mu.
func (a *Auth) accountKey(k string, now time.Time) bool {
	for _, p := range accountPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return strings.HasPrefix(k, "name:") && a.isRealName(k, now)
}

// pruneFails frees room in a.fails; the caller holds a.mu. Records whose wait
// is over go first; a real account's record only after accountKeep. If the
// map is still full (a flood of fresh addresses), the oldest address records
// go. Per-name and account records are never evicted that way: there are few
// real names, and flushing them would end the wait that protects an account.
// When names fill their share, noteFail stops adding new ones instead, so
// memory stays bounded and a flood of made-up names cannot remove the
// owner's wait.
func (a *Auth) pruneFails(now time.Time) {
	a.lastPruneAt = now
	for k, f := range a.fails {
		if idle := now.Sub(f.last); idle > MaxBackoff && (idle > accountKeep || !a.accountKey(k, now)) {
			delete(a.fails, k)
		}
	}
	if len(a.fails) < maxTracked {
		return
	}
	type kf struct {
		k    string
		last time.Time
	}
	var addrs []kf
	for k, f := range a.fails {
		if !protectedKey(k) && !a.accountKey(k, now) {
			addrs = append(addrs, kf{k, f.last})
		}
	}
	slices.SortFunc(addrs, func(x, y kf) int { return x.last.Compare(y.last) })
	for _, e := range addrs[:min(len(addrs)/5+1, len(addrs))] {
		delete(a.fails, e.k)
	}
}

// markFails reads the keys before noteFail; the caller holds a.mu.
func (a *Auth) markFails(keys ...string) []failMark {
	out := make([]failMark, 0, len(keys))
	for _, k := range keys {
		m := failMark{key: k}
		if f := a.fails[k]; f != nil {
			m.had, m.last = true, f.last
		}
		out = append(out, m)
	}
	return out
}

// undoFail takes back the failure noted up front when the check could not
// run at all (the media server did not answer), so an outage never locks
// people out or restarts their wait. Each key loses one count, as parallel
// attempts noted their own.
func (a *Auth) undoFail(marks []failMark) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.undoFailLocked(marks)
}

// undoFailLocked is undoFail for a caller that holds a.mu. A good sign-in
// uses it to take back only its own reservation on a shared address, so it
// never clears the count other people's failures left there.
func (a *Auth) undoFailLocked(marks []failMark) {
	for _, m := range marks {
		f := a.fails[m.key]
		if f == nil {
			continue
		}
		if f.n--; f.n <= 0 {
			delete(a.fails, m.key)
		} else if m.had {
			f.last = m.last
		}
	}
}
