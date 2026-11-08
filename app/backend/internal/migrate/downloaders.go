package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// qbitReader reads qBittorrent's connection details, folders and
// categories. Its password (and API key) is never read: the Snapshot says
// it needs the password again.
type qbitReader struct{}

// Kind returns the app this reader handles.
func (qbitReader) Kind() Kind { return QBittorrent }

var qbitFiles = []string{"qbittorrent.conf", "qbittorrent.ini", "categories.json"}

// login signs in with the typed user name and password (or uses the typed
// API key as a bearer token on 5.2 and later). The session cookie stays
// in this client only.
func (qbitReader) login(ctx context.Context, src Source) (*apiClient, error) {
	key := src.Key
	c, err := newAPIClient(src, func(req *http.Request) {
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	})
	if err != nil {
		return nil, err
	}
	jar, _ := cookiejar.New(nil)
	c.h.Jar = jar
	if key == "" {
		if src.Password == "" {
			return nil, ErrBadKey
		}
		_, _, _, err := c.do(ctx, http.MethodPost, "/api/v2/auth/login", nil, url.Values{"username": {src.Username}, "password": {src.Password}})
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (r qbitReader) version(ctx context.Context, c *apiClient) (string, error) {
	b, _, _, err := c.do(ctx, http.MethodGet, "/api/v2/app/version", nil, nil)
	if err != nil {
		if errors.Is(err, ErrBadAnswer) {
			return "", ErrWrongApp
		}
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" || len(v) > 40 || strings.ContainsAny(v, "<{") {
		return "", ErrWrongApp
	}
	return v, nil
}

// Probe checks the downloader answers and reports its version.
func (r qbitReader) Probe(ctx context.Context, src Source) (Info, error) {
	if src.Via() == ViaBackup {
		b, _, err := r.conf(src)
		if err != nil {
			return Info{}, err
		}
		b.close()
		return Info{Kind: QBittorrent, Via: ViaBackup, Notes: []string{"the settings file does not say which version wrote it"}}, nil
	}
	c, err := r.login(ctx, src)
	if err != nil {
		return Info{}, err
	}
	v, err := r.version(ctx, c)
	if err != nil {
		return Info{}, err
	}
	return Info{Kind: QBittorrent, Via: ViaAPI, Version: v}, nil
}

// conf opens the backup and reads qBittorrent's settings file.
func (qbitReader) conf(src Source) (*backup, iniFile, error) {
	b, err := openBackup(src, qbitFiles)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range []string{"qbittorrent.conf", "qbittorrent.ini"} {
		data, ok, err := b.read(n)
		if err != nil || !ok {
			continue
		}
		f := parseINI(data)
		if f["preferences"] == nil && f["bittorrent"] == nil {
			b.close()
			return nil, nil, ErrWrongApp
		}
		return b, f, nil
	}
	b.close()
	return nil, nil, ErrNotBackup
}

// Read fetches the downloader's setup into a Snapshot.
func (r qbitReader) Read(ctx context.Context, src Source) (*Snapshot, error) {
	d := &Downloader{NeedsPassword: true}
	s := &Snapshot{Downloader: d}
	if src.Via() == ViaBackup {
		b, f, err := r.conf(src)
		if err != nil {
			return nil, err
		}
		defer b.close()
		p, bt := f["preferences"], f["bittorrent"]
		d.Port = int(num(p[`webui\port`]))
		if d.Port == 0 {
			d.Port = 8080
		}
		d.Username = p[`webui\username`]
		d.TLS = boolean(p[`webui\https\enabled`])
		d.SavePath = firstString(bt[`session\defaultsavepath`], p[`downloads\savepath`])
		if data, ok, err := b.read("categories.json"); err == nil && ok {
			var cats map[string]map[string]any
			if decodeJSON(data, b.lim.MaxDepth, &cats) == nil {
				for name, c := range cats {
					d.Categories = append(d.Categories, Category{Name: name, Path: str(c["save_path"])})
				}
			} else {
				s.Notes = append(s.Notes, "categories.json could not be read")
			}
		}
	} else {
		c, err := r.login(ctx, src)
		if err != nil {
			return nil, err
		}
		if s.Version, err = r.version(ctx, c); err != nil {
			return nil, err
		}
		u, _ := url.Parse(src.URL)
		d.Host, d.TLS = u.Hostname(), u.Scheme == "https"
		d.Port, _ = strconv.Atoi(u.Port())
		d.URLBase = strings.TrimRight(u.Path, "/")
		var prefs map[string]any
		if err := c.getJSON(ctx, "/api/v2/app/preferences", nil, &prefs); err != nil {
			return nil, err
		}
		d.Username = firstString(src.Username, str(prefs["web_ui_username"]))
		d.SavePath = str(prefs["save_path"])
		var cats map[string]map[string]any
		if err := c.getJSON(ctx, "/api/v2/torrents/categories", nil, &cats); err != nil {
			return nil, err
		}
		for name, cat := range cats {
			d.Categories = append(d.Categories, Category{Name: name, Path: str(cat["savePath"])})
		}
	}
	sort.Slice(d.Categories, func(i, j int) bool { return d.Categories[i].Name < d.Categories[j].Name })
	return s, nil
}

func firstString(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// sabReader reads SABnzbd's connection details, folders, categories and
// usenet servers. Its keys and the servers' passwords are left behind.
type sabReader struct{}

// Kind returns the app this reader handles.
func (sabReader) Kind() Kind { return SABnzbd }

var sabFiles = []string{"sabnzbd.ini"}

func (sabReader) client(src Source) (*apiClient, error) { return newAPIClient(src, nil) }

// call runs one API mode; the key rides in the query, as SABnzbd wants,
// and never reaches an error's text (see apiClient.do).
func (sabReader) call(ctx context.Context, c *apiClient, key string, q url.Values, v any) error {
	q.Set("output", "json")
	if key != "" {
		q.Set("apikey", key)
	}
	b, _, _, err := c.do(ctx, http.MethodGet, "/api", q, nil)
	if err != nil {
		return err
	}
	if err := decodeJSON(b, c.lim.MaxDepth, v); err != nil {
		return err
	}
	// SABnzbd answers a wrong key with 200 and an error field.
	var e struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != nil && e.Error != false {
		return ErrBadKey
	}
	return nil
}

func (r sabReader) version(ctx context.Context, c *apiClient) (string, error) {
	var v map[string]any
	if err := r.call(ctx, c, "", url.Values{"mode": {"version"}}, &v); err != nil {
		if errors.Is(err, ErrBadAnswer) {
			return "", ErrWrongApp
		}
		return "", err
	}
	if str(v["version"]) == "" {
		return "", ErrWrongApp
	}
	return str(v["version"]), nil
}

func (r sabReader) ini(src Source) (iniFile, error) {
	b, err := openBackup(src, sabFiles)
	if err != nil {
		return nil, err
	}
	defer b.close()
	data, ok, err := b.read("sabnzbd.ini")
	if err != nil || !ok {
		return nil, ErrNotBackup
	}
	f := parseINI(data)
	if f["misc"] == nil {
		return nil, ErrWrongApp
	}
	return f, nil
}

// Probe checks the downloader answers and reports its version.
func (r sabReader) Probe(ctx context.Context, src Source) (Info, error) {
	if src.Via() == ViaBackup {
		f, err := r.ini(src)
		if err != nil {
			return Info{}, err
		}
		return Info{Kind: SABnzbd, Via: ViaBackup, Version: "settings " + f[""]["__version__"]}, nil
	}
	c, err := r.client(src)
	if err != nil {
		return Info{}, err
	}
	v, err := r.version(ctx, c)
	if err != nil {
		return Info{}, err
	}
	return Info{Kind: SABnzbd, Via: ViaAPI, Version: v}, nil
}

// Read fetches the downloader's setup into a Snapshot.
func (r sabReader) Read(ctx context.Context, src Source) (*Snapshot, error) {
	d := &Downloader{NeedsPassword: true}
	s := &Snapshot{Downloader: d}
	var misc map[string]any
	var cats, servers []map[string]any
	if src.Via() == ViaBackup {
		f, err := r.ini(src)
		if err != nil {
			return nil, err
		}
		s.Version = "settings " + f[""]["__version__"]
		misc = map[string]any{}
		for k, v := range f["misc"] {
			misc[k] = v
		}
		for _, sec := range f.subsections("categories") {
			m := map[string]any{"name": strings.TrimPrefix(sec, "categories/")}
			for k, v := range f[sec] {
				m[k] = v
			}
			cats = append(cats, m)
		}
		for _, sec := range f.subsections("servers") {
			m := map[string]any{"name": strings.TrimPrefix(sec, "servers/")}
			for k, v := range f[sec] {
				m[k] = v
			}
			servers = append(servers, m)
		}
	} else {
		c, err := r.client(src)
		if err != nil {
			return nil, err
		}
		if s.Version, err = r.version(ctx, c); err != nil {
			return nil, err
		}
		var got struct {
			Config struct {
				Misc       map[string]any   `json:"misc"`
				Categories []map[string]any `json:"categories"`
				Servers    []map[string]any `json:"servers"`
			} `json:"config"`
		}
		if err := r.call(ctx, c, src.Key, url.Values{"mode": {"get_config"}}, &got); err != nil {
			return nil, err
		}
		misc, cats, servers = got.Config.Misc, got.Config.Categories, got.Config.Servers
		if misc == nil {
			return nil, ErrBadAnswer
		}
		u, _ := url.Parse(src.URL)
		d.Host = u.Hostname()
	}
	if d.Host == "" {
		d.Host = str(misc["host"])
	}
	d.TLS = boolean(misc["enable_https"])
	d.Port = int(num(misc["port"]))
	if d.TLS && num(misc["https_port"]) != 0 {
		d.Port = int(num(misc["https_port"]))
	}
	d.URLBase = str(misc["url_base"])
	d.Username = str(misc["username"])
	d.SavePath, d.CompletePath = str(misc["download_dir"]), str(misc["complete_dir"])
	for _, m := range cats {
		d.Categories = append(d.Categories, Category{Name: str(m["name"]), Path: str(m["dir"]), Priority: str(m["priority"])})
	}
	sort.Slice(d.Categories, func(i, j int) bool { return d.Categories[i].Name < d.Categories[j].Name })
	for _, m := range servers {
		name := str(first(m, "displayname", "name", "host"))
		p := Provider{Name: name, Kind: "usenet", Enabled: m["enable"] == nil || boolean(m["enable"]), Priority: int(num(m["priority"]))}
		rest := map[string]any{}
		for k, v := range m {
			if k != "name" && k != "displayname" && k != "enable" && k != "priority" {
				rest[k] = v
			}
		}
		p.Settings, p.NeedsReentry = plainSettings(rest)
		d.Servers = append(d.Servers, p)
	}
	return s, nil
}
