package doctor

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// probeWait bounds the second look at an address.
const probeWait = 3 * time.Second

// probe is Probe; tests swap it out.
var probe = Probe

// Dial opens a connection the way the caller's own calls do, with its network
// policy checked on every address after DNS.
type Dial func(ctx context.Context, network, addr string) (net.Conn, error)

// Probe looks at rawURL once more in the other speech, for a call that failed
// without a cause: an http:// port that finishes a TLS handshake speaks
// https, and an https:// port that answers plain "HTTP/" speaks http.
// Anything else stays Unknown. It connects only through dial, so the
// caller's network policy holds; with no dial it does not look.
func Probe(rawURL string, dial Dial) Cause {
	if dial == nil {
		return Unknown
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return Unknown
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeWait)
	defer cancel()
	conn, err := dial(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return Unknown
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(probeWait))
	switch scheme {
	case "http":
		// Only whether it speaks TLS matters; nothing is sent after the handshake.
		tc := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: true}) //nolint:gosec // detects TLS only
		if tc.HandshakeContext(ctx) == nil {
			return UseHTTPS
		}
	case "https":
		if _, err := io.WriteString(conn, "HEAD / HTTP/1.0\r\nHost: "+u.Host+"\r\n\r\n"); err != nil {
			return Unknown
		}
		// nginx and Cloudflare answer plain text on their TLS port with a
		// page saying so: that port speaks https after all.
		b := make([]byte, 512)
		n, _ := io.ReadAtLeast(conn, b, 5)
		if n >= 5 && string(b[:5]) == "HTTP/" && !strings.Contains(strings.ToLower(string(b[:n])), "sent to https port") {
			return UseHTTP
		}
	}
	return Unknown
}

// apps are the apps an answer can name, each with the words that give it away
// in its headers or the start of its body.
var apps = []struct {
	name  string
	marks []string
}{
	{"jellyseerr", []string{"jellyseerr"}},
	{"overseerr", []string{"overseerr"}},
	{"sonarr", []string{"sonarr"}},
	{"radarr", []string{"radarr"}},
	{"lidarr", []string{"lidarr"}},
	{"readarr", []string{"readarr"}},
	{"whisparr", []string{"whisparr"}},
	{"prowlarr", []string{"prowlarr"}},
	{"bazarr", []string{"bazarr"}},
	{"jackett", []string{"jackett"}},
	{"jellyfin", []string{"jellyfin", "x-response-time-ms"}},
	{"emby", []string{"emby server", "x-emby-", "<title>emby"}},
	{"plex", []string{"x-plex-", "plex media server", "<mediacontainer"}},
	{"qbittorrent", []string{"qbittorrent", "vuetorrent"}},
	{"sabnzbd", []string{"sabnzbd"}},
	{"nzbget", []string{"nzbget"}},
	{"transmission", []string{"x-transmission-session-id", "<title>transmission", "realm=\"transmission"}},
	{"deluge", []string{"deluge"}},
	// HolyArrs's own security policy names its trailer player, even on a
	// bare 401.
	{"holyarrs", []string{"holyarrs", "youtube-nocookie.com"}},
	// Sonarr, Radarr, Prowlarr and the rest of that family send this header.
	{"servarr", []string{"x-application-version:"}},
	// An empty answer from a .NET app names only its web server: Jellyfin,
	// Emby and that family answer a wrong path or key this way.
	{"kestrel", []string{"server: kestrel"}},
}

// kestrelApps are the apps that answer as Kestrel, and servarrApps those of
// them that send the version header.
var (
	servarrApps = []string{"sonarr", "radarr", "lidarr", "readarr", "whisparr", "prowlarr"}
	kestrelApps = append([]string{"jellyfin", "emby", "jackett"}, servarrApps...)
)

// Who names the app that answered, from its headers and the start of its
// body; "" when nothing gives it away.
func Who(h http.Header, body []byte) string {
	var sb strings.Builder
	for k, vs := range h {
		sb.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	if len(body) > 4096 {
		body = body[:4096]
	}
	sb.Write(body)
	s := strings.ToLower(sb.String())
	for _, a := range apps {
		for _, m := range a.marks {
			if strings.Contains(s, m) {
				return a.name
			}
		}
	}
	return ""
}

// Other is a WrongApp mark when the answer names an app that is none of own,
// and nil otherwise. Callers ask it before reading a 401, 403 or 404 as a
// key or path problem.
func Other(h http.Header, body []byte, own ...string) error {
	n := Who(h, body)
	if n == "" {
		return nil
	}
	same := []string{n}
	switch n {
	case "servarr":
		same = servarrApps
	case "kestrel":
		same = kestrelApps
	}
	for _, o := range own {
		for _, s := range same {
			if strings.EqualFold(o, s) {
				return nil
			}
		}
	}
	return Mark(WrongApp, nil)
}
