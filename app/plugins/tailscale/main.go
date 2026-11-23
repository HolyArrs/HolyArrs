// Command tailscale is the HolyArrs Tailscale plugin. It joins the tailnet as
// its own app-only device, serves HTTPS on its ts.net name and hands every
// visit to HolyArrs with the forwarding headers removed and a per-start mark
// set, so HolyArrs always asks for sign-in. Its own state lives in
// $HOLYARRS_PLUGIN_DATA.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/envknob"
	"tailscale.com/tsnet"
)

// Environment HolyArrs passes in. The auth key is read once and dropped.
const (
	envData     = "HOLYARRS_PLUGIN_DATA"
	envTarget   = "HOLYARRS_PLUGIN_TARGET" // HolyArrs' local address, e.g. http://127.0.0.1:6262
	envMark     = "HOLYARRS_PLUGIN_MARK"   // per-start random token
	envLogin    = "HOLYARRS_TAILSCALE_LOGIN_SERVER"
	envHostname = "HOLYARRS_TAILSCALE_HOSTNAME"
	envNoUDP    = "HOLYARRS_PLUGIN_NO_UDP" // set by HolyArrs 1.2.1+: UDP is blocked
)

// stopWithin bounds a clean stop.
const stopWithin = 10 * time.Second

var now = time.Now

// tsName is this device's ts.net name, kept fresh by watchStatus.
var tsName atomic.Value

// authKeyFile is the optional one-time auth key HolyArrs writes (0600) in
// the data folder. It is read once and deleted.
const authKeyFile = "authkey"

func readAuthKey(data string) string {
	p := filepath.Join(data, authKeyFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	if err := os.Remove(p); err != nil {
		log.Printf("auth key file: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func main() {
	signOutFlag := flag.Bool("sign-out", false, "log out, wipe the Tailscale state and exit")
	loginServer := flag.String("login-server", os.Getenv(envLogin), "control server (Headscale); empty for Tailscale")
	hostname := flag.String("hostname", savedName(envOr(envHostname, "holyarrs")), "device name on the tailnet")
	flag.Parse()
	os.Unsetenv("TS_AUTHKEY")
	os.Unsetenv("TS_AUTH_KEY")
	// No log upload to Tailscale.
	os.Setenv("TS_NO_LOGS_NO_SUPPORT", "true")
	// HolyArrs blocks UDP for plugins, so use only Tailscale's relays (DERP
	// over HTTPS); without this tsnet fails to start when it cannot bind UDP.
	if os.Getenv(envNoUDP) == "1" {
		envknob.Setenv("TS_DEBUG_ALWAYS_USE_DERP", "true")
	}

	data := os.Getenv(envData)
	if data == "" {
		log.Fatalf("%s is not set", envData)
	}
	if err := os.MkdirAll(stateDir(data), 0o700); err != nil {
		log.Fatal(err)
	}
	authKey := readAuthKey(data)
	srv := &tsnet.Server{
		Dir:        stateDir(data),
		Hostname:   *hostname,
		ControlURL: *loginServer,
		AuthKey:    authKey,
		Logf:       func(string, ...any) {}, // backend chatter off; may hold login links
		UserLogf:   func(f string, a ...any) { log.Print(redact(fmt.Sprintf(f, a...))) },
	}

	if *signOutFlag {
		srv.AuthKey = ""
		ctx, cancel := context.WithTimeout(context.Background(), stopWithin)
		defer cancel()
		logoutErr, wipeErr := signOut(ctx, data, func(ctx context.Context) error {
			if err := srv.Start(); err != nil {
				return err
			}
			lc, err := srv.LocalClient()
			if err != nil {
				return err
			}
			err = lc.Logout(ctx)
			srv.Close()
			return err
		})
		if logoutErr != nil {
			log.Printf("logout failed (%v); remove the device in Tailscale's admin page", logoutErr)
		}
		if wipeErr != nil {
			log.Fatal(wipeErr)
		}
		cancel()
		os.Exit(signOutCode(logoutErr))
	}

	target, err := parseTarget(os.Getenv(envTarget))
	if err != nil {
		log.Fatal(err)
	}
	mark := os.Getenv(envMark)
	if len(mark) < 16 {
		log.Fatalf("%s must be a random token of at least 16 characters", envMark)
	}
	os.Unsetenv(envMark)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := run(ctx, srv, data, target, mark); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, srv *tsnet.Server, data string, target *url.URL, mark string) error {
	_ = writeStatus(data, Status{State: StateNotConnected, Updated: now()})
	if err := srv.Start(); err != nil {
		return err
	}
	lc, err := srv.LocalClient()
	if err != nil {
		srv.Close()
		return err
	}
	// ListenTLS waits until the device is signed in, so status (and the
	// sign-in link) must be watched before it.
	go watchStatus(ctx, lc, data)
	ln, err := listen(srv)
	if err != nil {
		srv.Close()
		return err
	}
	who := func(r *http.Request) (string, string) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w, err := lc.WhoIs(c, r.RemoteAddr)
		if err != nil || w == nil {
			return "", ""
		}
		u, d := "", ""
		if w.UserProfile != nil {
			u = w.UserProfile.LoginName
		}
		if w.Node != nil {
			d = w.Node.ComputedName
		}
		return u, d
	}
	hs := &http.Server{
		Handler:           newProxy(target, mark, who, func() string { n, _ := tsName.Load().(string); return n }),
		ReadHeaderTimeout: 20 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(ln) }()

	select {
	case <-ctx.Done():
	case err = <-served:
	}
	sctx, cancel := context.WithTimeout(context.Background(), stopWithin)
	defer cancel()
	done := make(chan struct{})
	go func() {
		hs.Shutdown(sctx)
		srv.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-sctx.Done():
		log.Print("stop took too long; exiting")
	}
	return err
}

// watchStatus refreshes status.json every few seconds from Tailscale's own state.
func watchStatus(ctx context.Context, lc *local.Client, data string) {
	had := false
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		st, err := lc.Status(c)
		cancel()
		if err == nil {
			s := Status{Version: st.Version, Updated: now()}
			f := facts{backend: st.BackendState, authURL: st.AuthURL, hadNodeKey: had}
			if st.Self != nil {
				s.DeviceName = st.Self.HostName
				s.TSNetName = strings.TrimSuffix(st.Self.DNSName, ".")
				tsName.Store(s.TSNetName)
				if st.Self.KeyExpiry != nil {
					f.keyExpiry = *st.Self.KeyExpiry
					s.LoginExpiry = *st.Self.KeyExpiry
					f.keyExpired = !f.keyExpiry.IsZero() && f.keyExpiry.Before(now())
				}
			}
			s.State = pickState(f, now())
			if s.State == StateNeedsSignIn {
				s.LoginURL = st.AuthURL
			}
			if s.State == StateConnected || s.State == StateExpiring {
				had = true
				for _, p := range st.Peer {
					if !p.Active {
						continue
					}
					if p.CurAddr != "" {
						s.Connection = "direct"
						break
					}
					if p.Relay != "" || p.PeerRelay != "" {
						s.Connection = "relayed"
					}
				}
			}
			if err := writeStatus(data, s); err != nil {
				log.Printf("status: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// parseTarget accepts only a plain http(s) address with a host.
func parseTarget(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%s must be HolyArrs' local address, like http://127.0.0.1:6262", envTarget)
	}
	return u, nil
}

// redact hides every https link (login links, Headscale addresses) and keys
// in log lines.
func redact(s string) string {
	for _, w := range strings.Fields(s) {
		if strings.Contains(w, "https://") || strings.Contains(w, "tskey-") {
			s = strings.ReplaceAll(s, w, "[hidden]")
		}
	}
	return s
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// listen serves HTTPS on :443 when the tailnet has HTTPS certificates turned on.
// New tailnets have them off, and ListenTLS then fails; fall back to plain HTTP
// on :80, which is still end-to-end encrypted by Tailscale itself.
func listen(srv *tsnet.Server) (net.Listener, error) {
	ln, err := srv.ListenTLS("tcp", ":443")
	if err == nil {
		return ln, nil
	}
	if !httpsOff(err) {
		return nil, err
	}
	log.Printf("HTTPS is off for this tailnet, so HolyArrs is served over plain HTTP on the tailnet (Tailscale still encrypts it): %v", err)
	return srv.Listen("tcp", ":80")
}

// httpsOffText is part of the error tsnet's ListenTLS returns when the
// tailnet has HTTPS certificates off (pinned by a test against tsnet's source).
const httpsOffText = "you must enable HTTPS in the admin panel"

func httpsOff(err error) bool { return err != nil && strings.Contains(err.Error(), httpsOffText) }

var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// savedName is the device name the owner picked in HolyArrs (the hostname
// file in the data folder), or def.
func savedName(def string) string {
	b, err := os.ReadFile(filepath.Join(os.Getenv(envData), "hostname"))
	if n := strings.TrimSpace(string(b)); err == nil && nameRE.MatchString(n) {
		return n
	}
	return def
}
