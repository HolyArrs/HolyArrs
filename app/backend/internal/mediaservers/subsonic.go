package mediaservers

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Subsonic-style music servers (Navidrome, Gonic, Airsonic, Subsonic) have
// no per-folder scan: HolyArrs asks for one library scan per batch, the way
// the old music app did. They hold a user name and password instead of a
// token; both are kept as one vault secret, the name first, then a new line.

// ErrNoUser asks for the user name a Subsonic-style server signs in with.
var ErrNoUser = errors.New("type the user name and password the music server signs in with")

// isMusicOnly says m is a music server that is only told about new music,
// never asked for people, watch state or labels.
func isMusicOnly(m domain.MediaServer) bool { return m.Kind == "subsonic" }

// subsonicSecret is what the vault keeps for a Subsonic-style server.
func subsonicSecret(user, password string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" || password == "" || strings.ContainsAny(user, "\r\n") {
		return "", ErrNoUser
	}
	return user + "\n" + password, nil
}

// ErrPlainHTTP refuses sending the password itself over plain http to a
// host outside the local network, where anyone on the way could read it.
var ErrPlainHTTP = errors.New("the password is only sent as it is over https or to a local address; use an https address or turn off Plain password")

// errTokenRefused says the server did not take the salted token, and how to
// sign in instead.
var errTokenRefused = errors.New("the music server does not take the safer salted sign-in (it may check passwords with LDAP); turn on Plain password (for LDAP) for this server to send the password itself")

// noAuth asks doStream to leave the sign-in out (the extension list is open
// to everyone).
type noAuth struct{}

// asForm asks doStream to send the call as a form POST, keeping the sign-in
// out of the address.
type asForm struct{}

// plainSafe says the password itself may be sent to raw: over https, or
// to this machine or the local network.
func plainSafe(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return true // a container or machine name on the local network
	}
	for _, end := range []string{".localhost", ".local", ".lan", ".home.arpa", ".internal"} {
		if strings.HasSuffix(host, end) {
			return true
		}
	}
	return false
}

// subsonicParams is the sign-in part of a Subsonic call: the salted token
// with a new salt each call, or the password itself only when the owner
// turned Plain password on.
func subsonicParams(secret string, plain bool) url.Values {
	user, pw, _ := strings.Cut(secret, "\n")
	q := url.Values{"u": {user}, "v": {"1.13.0"}, "c": {"HolyArrs"}, "f": {"json"}}
	if plain {
		q.Set("v", "1.12.0")
		q.Set("p", "enc:"+hex.EncodeToString([]byte(pw)))
		return q
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	salt := hex.EncodeToString(b)
	sum := md5.Sum([]byte(pw + salt))
	q.Set("s", salt)
	q.Set("t", hex.EncodeToString(sum[:]))
	return q
}

// formPostKey is one server at one address, so a new address is asked again.
type formPostKey struct {
	id  int64
	url string
}

// subsonicFormPost says the server takes form POSTs (OpenSubsonic
// formPost), so the sign-in stays out of the address. Asked once per server
// and address; a failed ask is asked again next time.
func (l *Linker) subsonicFormPost(ctx context.Context, m domain.MediaServer) bool {
	k := formPostKey{m.ID, m.URL}
	if v, ok := l.formPost.Load(k); ok {
		return v.(bool)
	}
	b, err := l.do(context.WithValue(ctx, noAuth{}, true), m, http.MethodGet, "/rest/getOpenSubsonicExtensions", nil)
	if err != nil {
		return false
	}
	var a struct {
		R *struct {
			Status string `json:"status"`
			Ext    []struct {
				Name string `json:"name"`
			} `json:"openSubsonicExtensions"`
		} `json:"subsonic-response"`
	}
	if json.Unmarshal(b, &a) != nil || a.R == nil {
		return false
	}
	yes := false
	for _, e := range a.R.Ext {
		yes = yes || e.Name == "formPost"
	}
	l.formPost.Store(k, yes)
	return yes
}

// subsonicCall calls one Subsonic method and reads the answer's status.
// A refused sign-in or a page that is not a Subsonic answer becomes the
// doctor's plain cause.
func (l *Linker) subsonicCall(ctx context.Context, m domain.MediaServer, method string) (string, error) {
	if m.PlainPassword && !plainSafe(m.URL) {
		return "", ErrPlainHTTP
	}
	if l.subsonicFormPost(ctx, m) {
		ctx = context.WithValue(ctx, asForm{}, true)
	}
	b, err := l.do(ctx, m, http.MethodGet, "/rest/"+method, nil)
	if err != nil {
		return "", err
	}
	var a struct {
		R *struct {
			Status        string `json:"status"`
			Version       string `json:"version"`
			Type          string `json:"type"`
			ServerVersion string `json:"serverVersion"`
			Error         *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"subsonic-response"`
	}
	if json.Unmarshal(b, &a) != nil || a.R == nil {
		return "", doctor.Mark(doctor.WrongApp, nil)
	}
	if a.R.Status != "ok" {
		code := 0
		if a.R.Error != nil {
			code = a.R.Error.Code
		}
		switch code {
		case 41, 42:
			// Salted token not taken. Never fall back to the password
			// itself on our own: the owner turns that on.
			if !m.PlainPassword {
				return "", errTokenRefused
			}
			return "", doctor.Mark(doctor.Auth, nil)
		case 40, 44:
			return "", doctor.Mark(doctor.Auth, nil)
		case 50:
			return "", errors.New("the music server's user may not start a library scan; give it the admin role")
		}
		return "", errors.New("the music server refused the call")
	}
	name := a.R.Type
	if name == "" {
		name = "Subsonic"
	}
	ver := a.R.ServerVersion
	if ver == "" {
		ver = a.R.Version
	}
	return strings.TrimSpace(strings.ToUpper(name[:1]) + name[1:] + " " + ver), nil
}
