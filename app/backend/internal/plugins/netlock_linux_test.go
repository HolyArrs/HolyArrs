//go:build linux

package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// requireNetLock fails (never skips) when the kernel cannot give the network
// lock: these tests are the proof that it works.
func requireNetLock(t *testing.T) {
	t.Helper()
	if landlockABI() < 1 {
		t.Fatal("this kernel has no Landlock; the plugin sandbox tests need Linux 5.13 or newer")
	}
	if ok, why := netLockProbe(); !ok {
		t.Fatalf("this system cannot give the network lock (%s); the plugin sandbox tests need Linux 5.9 or newer with seccomp user-notify and process_vm_readv allowed", why)
	}
}

// fakeNet answers names with public test addresses and dials them to local servers.
type fakeNet struct {
	names map[string][]netip.Addr
	to    map[netip.Addr]string // address -> local server
	dials []string
}

func (f *fakeNet) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f.names[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func (f *fakeNet) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return nil, err
	}
	f.dials = append(f.dials, addr)
	local, ok := f.to[ap.Addr()]
	if !ok {
		return nil, fmt.Errorf("fake net: nothing at %s", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, local)
}

type netRig struct {
	t      *testing.T
	data   string
	probe  string
	proxy  *Proxy
	handle *ProxyHandle
	plain  *httptest.Server // the listed host's web server on port 80
	tls    *httptest.Server // the same on 443
	self   *httptest.Server // the app
	net    *fakeNet
}

var (
	listedIP = netip.MustParseAddr("93.184.216.34")
	lanIP    = netip.MustParseAddr("10.0.0.7")
)

func newNetRig(t *testing.T, owner ...string) *netRig {
	requireNetLock(t)
	_, data, probe := probeDirs(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello from "+r.Host) })
	rig := &netRig{t: t, data: data, probe: probe, plain: httptest.NewServer(ok), tls: httptest.NewTLSServer(ok), self: httptest.NewServer(ok)}
	t.Cleanup(func() { rig.plain.Close(); rig.tls.Close(); rig.self.Close() })
	rig.net = &fakeNet{names: map[string][]netip.Addr{
		"listed.example.com":     {listedIP},
		"a.wild.example.com":     {listedIP},
		"rebind.example.com":     {listedIP, netip.MustParseAddr("127.0.0.1")},
		"lan.example.com":        {lanIP},
		"unlisted.example.com":   {listedIP},
		"headscale.example.home": {lanIP},
	}, to: map[netip.Addr]string{}}
	rig.proxy = &Proxy{Resolver: rig.net, Own: func() []netip.Addr { return nil }}
	rig.proxy.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		ap := netip.MustParseAddrPort(addr)
		srv := rig.plain
		if ap.Port() == 443 {
			srv = rig.tls
		}
		rig.net.to[ap.Addr()] = srv.Listener.Addr().String()
		return rig.net.dial(ctx, network, addr)
	}
	h, err := rig.proxy.Register("nettest", []string{"listed.example.com", "*.wild.example.com", "rebind.example.com", "lan.example.com"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	rig.handle = h
	return rig
}

// run starts the probe with the network lock and returns its error and output.
func (r *netRig) run(args ...string) (string, error) {
	r.t.Helper()
	self := strings.TrimPrefix(r.self.URL, "http://")
	out := filepath.Join(r.data, "out.txt")
	os.Remove(out)
	spec := Spec{ID: "nettest", Program: r.probe, Args: args, DataDir: r.data, Proxy: r.handle, SelfAddr: self,
		Env: r.handle.Env(self)}
	// HOLYARRS_HELPER runs the helper from another binary, such as the real server.
	p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: true, Helper: os.Getenv("HOLYARRS_HELPER")}}).Start(context.Background(), spec)
	if err != nil {
		r.t.Fatalf("start %v: %v", args, err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		p.Stop()
		r.t.Fatalf("probe %v hung", args)
	}
	return "", err
}

func TestNetLockListedHostThroughProxy(t *testing.T) {
	r := newNetRig(t)
	if _, err := r.run("httpget", "https://listed.example.com/"); err != nil {
		t.Errorf("listed host over HTTPS_PROXY: %v", err)
	}
	if _, err := r.run("httpget", "http://listed.example.com/"); err != nil {
		t.Errorf("listed host over plain http proxy: %v", err)
	}
	if _, err := r.run("httpget", "https://a.wild.example.com/"); err != nil {
		t.Errorf("wildcard host: %v", err)
	}
}

func TestNetLockRefusesUnlisted(t *testing.T) {
	r := newNetRig(t)
	for _, c := range [][]string{
		{"httpget", "https://unlisted.example.com/"},
		{"httpget", "https://wild.example.com/"}, // *.x never matches x itself
		{"httpget", "https://93.184.216.34/"},    // a raw IP, even one a listed name has
		{"httpget", "https://[2606:2800:220:1::1]/"},
		{"httpget", "https://listed.example.com:22/"}, // only 443 and 80
		{"httpget", "https://lan.example.com/"},       // a listed name that points at the LAN
		{"httpget", "https://rebind.example.com/"},    // one answer is loopback
		{"tcp", "1.1.1.1:443"},                        // straight to an address: ends at the proxy, which says nothing
		{"tls", "1.1.1.1:443", "unlisted.example.com"},
		{"tls", "1.1.1.1:443", ""},
		{"tcp", "[2606:4700::1111]:80"},
	} {
		if _, err := r.run(c...); err == nil {
			t.Errorf("%v worked; want refused", c)
		}
	}
	if n, recent := r.proxy.Blocked("nettest"); n == 0 || len(recent) == 0 {
		t.Errorf("no blocked connections counted: %d %v", n, recent)
	}
	for _, d := range r.net.dials {
		if strings.HasPrefix(d, "127.") {
			t.Errorf("proxy dialed loopback %s", d)
		}
	}
}

func TestNetLockSNIForDirectTLS(t *testing.T) {
	r := newNetRig(t)
	// A client that ignores the proxy settings still reaches a listed name by its server name.
	if _, err := r.run("tls", "1.1.1.1:443", "listed.example.com"); err != nil {
		t.Errorf("direct TLS with a listed server name: %v", err)
	}
}

func TestNetLockReachesApp(t *testing.T) {
	r := newNetRig(t)
	if _, err := r.run("httpget", r.self.URL+"/"); err != nil {
		t.Errorf("app address: %v", err)
	}
}

func TestNetLockRefusesOtherSockets(t *testing.T) {
	r := newNetRig(t)
	for _, c := range [][]string{
		{"udp"},
		{"socket", fmt.Sprint(unix.AF_INET), fmt.Sprint(unix.SOCK_DGRAM)},
		{"socket", fmt.Sprint(unix.AF_INET6), fmt.Sprint(unix.SOCK_DGRAM)},
		{"socket", fmt.Sprint(unix.AF_INET), fmt.Sprint(unix.SOCK_RAW)},
		{"socket", fmt.Sprint(unix.AF_PACKET), fmt.Sprint(unix.SOCK_RAW)},
		{"socket", fmt.Sprint(unix.AF_VSOCK), fmt.Sprint(unix.SOCK_STREAM)},
		{"iouring"},
		{"unix", "@holyarrs-test"},
		{"unix", "/var/run/docker.sock"},
	} {
		if _, err := r.run(c...); err == nil {
			t.Errorf("%v worked; want refused", c)
		}
	}
	if _, err := r.run("socket", fmt.Sprint(unix.AF_UNIX), fmt.Sprint(unix.SOCK_STREAM)); err != nil {
		t.Errorf("unix socket refused: %v", err)
	}
}

func TestNetLockOwnerHostMayReachLAN(t *testing.T) {
	r := newNetRig(t, "headscale.example.home:8080")
	r.net.names["headscale.example.home"] = []netip.Addr{lanIP}
	if _, err := r.run("httpget", "http://headscale.example.home/"); err != nil {
		t.Errorf("owner-added LAN host: %v", err)
	}
}

func TestNoHostsNoNetwork(t *testing.T) {
	requireNetLock(t)
	_, data, probe := probeDirs(t)
	for _, c := range [][]string{
		{"socket", fmt.Sprint(unix.AF_INET), fmt.Sprint(unix.SOCK_STREAM)},
		{"socket", fmt.Sprint(unix.AF_INET6), fmt.Sprint(unix.SOCK_STREAM)},
		{"tcp", "1.1.1.1:443"},
		{"unix", "@holyarrs-test"},
	} {
		p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: true}}).Start(context.Background(), Spec{ID: "nonet", Program: probe, Args: c, DataDir: data})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Wait(); err == nil {
			t.Errorf("no hosts: %v worked", c)
		}
	}
}

func TestNoNetLockNeedsOptIn(t *testing.T) {
	_, data, probe := probeDirs(t)
	proxy := &Proxy{}
	h, err := proxy.Register("optnet", []string{"listed.example.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	spec := Spec{ID: "optnet", Program: probe, Args: []string{"udp"}, DataDir: data, Proxy: h}
	if _, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: false}}).Start(context.Background(), spec); !errors.Is(err, ErrNoNetLock) {
		t.Fatalf("no lock, no opt-in: %v, want ErrNoNetLock", err)
	}
	spec.AllowNoNetLock = true
	p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: false}}).Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("no lock, opt-in: %v", err)
	}
	udpErr := p.Wait()
	// Where seccomp filters load at all, UDP stays blocked without the lock too.
	plain, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{ID: "optnet2", Program: probe, Args: []string{"read", "/etc/hosts"}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Wait() == nil && udpErr == nil {
		t.Error("UDP worked without the network lock; it must stay blocked")
	}
}

func TestFolderGrants(t *testing.T) {
	requireNetLock(t)
	root, data, probe := probeDirs(t)
	ro, rw, outside := filepath.Join(root, "music"), filepath.Join(root, "downloads"), filepath.Join(root, "outside")
	for _, d := range []string{ro, rw, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(ro, "song.flac"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o600)
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(rw, "link")); err != nil {
		t.Fatal(err)
	}
	grant := func(path, mode string) GrantedFolder {
		dev, ino, err := folderID(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := openFolder(path, dev, ino)
		if err != nil {
			t.Fatal(err)
		}
		return GrantedFolder{Name: filepath.Base(path), Path: path, Mode: mode, File: f}
	}
	run := func(args ...string) error {
		folders := []GrantedFolder{grant(ro, FolderRead), grant(rw, FolderReadWrite)}
		p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: true}}).Start(context.Background(), Spec{ID: "folders", Program: probe, Args: args, DataDir: data, Folders: folders})
		for _, f := range folders {
			f.File.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		return p.Wait()
	}
	if err := run("read", filepath.Join(ro, "song.flac")); err != nil {
		t.Errorf("read in read-only folder: %v", err)
	}
	if err := run("write", filepath.Join(ro, "new.txt")); err == nil {
		t.Error("write in read-only folder worked")
	}
	if err := run("write", filepath.Join(rw, "new.txt")); err != nil {
		t.Errorf("write in read-write folder: %v", err)
	}
	if err := run("symlink", "/etc/passwd", filepath.Join(rw, "planted")); err == nil {
		t.Error("symlink in read-write folder worked")
	}
	if err := run("read", filepath.Join(rw, "link")); err == nil {
		t.Error("symlink inside the folder pointing outside was followed")
	}
	if err := run("read", filepath.Join(outside, "secret.txt")); err == nil {
		t.Error("read outside the granted folders worked")
	}
}

func TestOpenFolderRefusesSwapAndLinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "music")
	os.Mkdir(dir, 0o700)
	dev, ino, err := folderID(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := openFolder(dir, dev, ino)
	if err != nil {
		t.Fatalf("same folder: %v", err)
	}
	f.Close()
	os.Rename(dir, dir+".old")
	os.Mkdir(dir, 0o700)
	if _, err := openFolder(dir, dev, ino); err == nil {
		t.Error("a new folder in the same place was accepted")
	}
	os.Remove(dir)
	os.Symlink(dir+".old", dir)
	if _, err := openFolder(dir, dev, ino); err == nil {
		t.Error("a link to the old folder was followed")
	}
}

func TestCheckFolderRefusesSystemAndConfig(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	media := filepath.Join(root, "media")
	os.MkdirAll(filepath.Join(cfg, "plugins"), 0o700)
	os.MkdirAll(media, 0o700)
	protected, inside := []string{"/usr/local/bin/holyarrs"}, []string{cfg, "/proc", "/sys", "/dev", "/etc", "/var/run"}
	for _, p := range []string{"/", "/etc", "/var/run/x", "/proc", cfg, filepath.Join(cfg, "plugins"), root, "/usr", "relative/path"} {
		if _, err := checkFolder(p, protected, inside); err == nil {
			t.Errorf("%s was accepted", p)
		}
	}
	if got, err := checkFolder(media, protected, inside); err != nil || got != media {
		t.Errorf("media folder: %q %v", got, err)
	}
	os.Symlink("/etc", filepath.Join(media, "etc-link"))
	if _, err := checkFolder(filepath.Join(media, "etc-link"), protected, inside); err == nil {
		t.Error("a link to /etc was accepted")
	}
}

func TestNetLockProbeWorksHere(t *testing.T) {
	requireNetLock(t)
}
