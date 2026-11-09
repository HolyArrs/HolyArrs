package migrate

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// bazarrReader reads Bazarr's subtitle setup: enabled languages, language
// profiles, subtitle sites and the default profiles. Sites' logins and
// keys are left behind.
type bazarrReader struct{}

// Kind returns the app this reader handles.
func (bazarrReader) Kind() Kind { return Bazarr }

func (bazarrReader) client(src Source) (*apiClient, error) {
	key := src.Key
	return newAPIClient(src, func(req *http.Request) { req.Header.Set("X-API-KEY", key) })
}

// unwrap takes Bazarr's {"data": ...} wrapper off when it is there.
func unwrap(v any) any {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"]; ok && len(m) <= 2 {
			return d
		}
	}
	return v
}

func (r bazarrReader) status(ctx context.Context, c *apiClient) (map[string]any, error) {
	var got any
	if err := c.getJSON(ctx, "/api/system/status", nil, &got); err != nil {
		if errors.Is(err, ErrBadAnswer) {
			return nil, ErrWrongApp
		}
		return nil, err
	}
	st := obj(unwrap(got))
	if st == nil || st["bazarr_version"] == nil {
		return nil, ErrWrongApp
	}
	return st, nil
}

var bazarrFiles = []string{"bazarr.db", "config.yaml", "config.ini"}

// Probe checks Bazarr answers and reports its version.
func (r bazarrReader) Probe(ctx context.Context, src Source) (Info, error) {
	if src.Via() == ViaBackup {
		b, err := openBackup(src, bazarrFiles)
		if err != nil {
			return Info{}, err
		}
		defer b.close()
		if !b.has("bazarr.db") {
			return Info{}, ErrWrongApp
		}
		db, err := b.db("bazarr.db")
		if err != nil {
			return Info{}, err
		}
		defer db.Close()
		var notes []string
		t := table{db: db, lim: b.lim, notes: &notes}
		if !t.exists(ctx, "table_languages_profiles") {
			return Info{}, ErrWrongApp
		}
		return Info{Kind: Bazarr, Via: ViaBackup, Version: alembic(ctx, t)}, nil
	}
	c, err := r.client(src)
	if err != nil {
		return Info{}, err
	}
	st, err := r.status(ctx, c)
	if err != nil {
		return Info{}, err
	}
	return Info{Kind: Bazarr, Via: ViaAPI, Version: str(st["bazarr_version"])}, nil
}

// alembic reads the database's migration name.
func alembic(ctx context.Context, t table) string {
	rows, err := t.rows(ctx, "alembic_version", "version_num")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return "database " + str(rows[0]["version_num"])
}

// Read fetches Bazarr's setup into a Snapshot.
func (r bazarrReader) Read(ctx context.Context, src Source) (*Snapshot, error) {
	s := &Snapshot{Subtitles: &SubtitleSetup{}}
	var langs, profiles []map[string]any
	var cfg map[string]any
	if src.Via() == ViaBackup {
		b, err := openBackup(src, bazarrFiles)
		if err != nil {
			return nil, err
		}
		defer b.close()
		if !b.has("bazarr.db") {
			return nil, ErrNotBackup
		}
		db, err := b.db("bazarr.db")
		if err != nil {
			return nil, err
		}
		defer db.Close()
		t := table{db: db, lim: b.lim, notes: &s.Notes}
		if !t.exists(ctx, "table_languages_profiles") && !t.exists(ctx, "table_settings_languages") {
			return nil, ErrWrongApp
		}
		s.Version = alembic(ctx, t)
		if langs, err = t.all(ctx, "table_settings_languages"); err != nil {
			return nil, err
		}
		if profiles, err = t.all(ctx, "table_languages_profiles"); err != nil {
			return nil, err
		}
		if y, ok, err := b.read("config.yaml"); err == nil && ok {
			cfg = parseYAML(y)
		} else if in, ok, err := b.read("config.ini"); err == nil && ok {
			cfg = iniToFlat(parseINI(in))
		} else {
			s.Notes = append(s.Notes, "the backup has no settings file; subtitle sites were not read")
		}
	} else {
		c, err := r.client(src)
		if err != nil {
			return nil, err
		}
		st, err := r.status(ctx, c)
		if err != nil {
			return nil, err
		}
		s.Version = str(st["bazarr_version"])
		var got any
		if err := c.getJSON(ctx, "/api/system/languages", nil, &got); err != nil {
			return nil, err
		}
		for _, e := range list(unwrap(got)) {
			langs = append(langs, lower(obj(e)))
		}
		got = nil
		if err := c.getJSON(ctx, "/api/system/languages/profiles", nil, &got); err != nil {
			return nil, err
		}
		for _, e := range list(unwrap(got)) {
			profiles = append(profiles, lower(obj(e)))
		}
		got = nil
		if err := c.getJSON(ctx, "/api/system/settings", nil, &got); err != nil {
			if !errors.Is(err, ErrBadAnswer) {
				return nil, err
			}
			s.Notes = append(s.Notes, "the app did not hand out its settings; subtitle sites were not read")
		} else {
			cfg = flatten("", obj(unwrap(got)), map[string]any{})
		}
	}
	for _, m := range langs {
		if boolean(m["enabled"]) {
			code := str(m["code2"])
			if code == "" {
				code = str(m["code3"])
			}
			s.Subtitles.Languages = append(s.Subtitles.Languages, code)
		}
	}
	sort.Strings(s.Subtitles.Languages)
	for _, m := range profiles {
		s.Subtitles.Profiles = append(s.Subtitles.Profiles, subtitleProfile(m))
	}
	if cfg != nil {
		r.sites(cfg, s.Subtitles)
	}
	return s, nil
}

// subtitleProfile reads one language profile (items are JSON text in a
// backup, a list in the API; flags may be "True" text).
func subtitleProfile(m map[string]any) SubtitleProfile {
	p := SubtitleProfile{ID: num(first(m, "profileid", "id")), Name: str(m["name"]), Tag: str(m["tag"])}
	cutoff := num(m["cutoff"])
	for _, it := range list(m["items"]) {
		im := lower(obj(it))
		l := SubtitleLanguage{Language: str(im["language"]), Forced: boolean(im["forced"]), HI: boolean(im["hi"])}
		p.Languages = append(p.Languages, l)
		if cutoff != 0 && num(im["id"]) == cutoff {
			p.Cutoff = l.Language
		}
	}
	if cutoff == 65535 {
		p.Cutoff = "any"
	}
	return p
}

// sites reads the enabled subtitle sites and their plain settings.
func (bazarrReader) sites(cfg map[string]any, out *SubtitleSetup) {
	for _, name := range stringList(cfg["general.enabled_providers"]) {
		set := map[string]any{}
		for k, v := range cfg {
			if rest, ok := strings.CutPrefix(k, name+"."); ok {
				set[rest] = v
			}
		}
		p := Provider{Name: name, Kind: name, Enabled: true}
		p.Settings, p.NeedsReentry = plainSettings(set)
		out.Providers = append(out.Providers, p)
	}
	d := map[string]any{}
	for _, k := range []string{"serie_default_profile", "movie_default_profile", "serie_default_enabled", "movie_default_enabled", "use_embedded_subs", "adaptive_searching"} {
		if v, ok := cfg["general."+k]; ok {
			d[k] = v
		}
	}
	out.Defaults, _ = plainSettings(d)
}

// stringList reads a list that may be a real list, a Python-style text
// list ("['a', 'b']") or a comma list.
func stringList(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if s := str(e); s != "" {
				out = append(out, s)
			}
		}
	case string:
		x = strings.Trim(strings.TrimSpace(x), "[]")
		for _, e := range strings.Split(x, ",") {
			if e = strings.Trim(strings.TrimSpace(e), `'"`); e != "" {
				out = append(out, e)
			}
		}
	}
	return out
}

// flatten joins nested settings into "section.key" names.
func flatten(prefix string, m map[string]any, out map[string]any) map[string]any {
	for k, v := range m {
		key := strings.ToLower(k)
		if prefix != "" {
			key = prefix + "." + key
		}
		if sub, ok := v.(map[string]any); ok {
			flatten(key, sub, out)
			continue
		}
		out[key] = v
	}
	return out
}

// iniToFlat turns an INI file into "section.key" names; values that look
// like numbers or booleans are read as such.
func iniToFlat(f iniFile) map[string]any {
	out := map[string]any{}
	for sec, kv := range f {
		for k, v := range kv {
			var val any = v
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				val = n
			} else if b, err := strconv.ParseBool(v); err == nil {
				val = b
			}
			out[sec+"."+k] = val
		}
	}
	return out
}
