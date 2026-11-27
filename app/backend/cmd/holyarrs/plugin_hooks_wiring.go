package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// newPluginHooks makes the provider-hook registry. A plugin's grants are
// those of its live token, so they end the moment the token does.
func newPluginHooks() *hooks.Registry {
	b := hooks.NewBroker()
	b.Granted = accounts.PluginTokenScopes
	b.Covers = accounts.PluginScopesCover
	reg := hooks.NewRegistry(b)
	reg.Alive = func(id string) context.Context {
		if c := accounts.PluginTokenContext(id); c != nil {
			return c
		}
		return nil
	}
	sources.PluginTransport = func(id string) http.RoundTripper { return &hooks.ReleaseTransport{Broker: b, Plugin: id} }
	return reg
}

// wireSourcePlugins lets plugins solve bot checks for Sources and serve
// subtitle sites. It runs before the job queue starts, so a make Job a stop
// interrupted finds plugin sites at once; engine is nil until the plugins
// start, and until then no plugin is asked anything.
func wireSourcePlugins(b *hooks.Broker, engine func() *plugins.Engine, db database.Querier) {
	wireBotCheck(b, func(id string) []string {
		if e := engine(); e != nil {
			return e.ReachHosts(id)
		}
		return nil // no plugin polls before the engine is set
	})
	wireSubtitleSites(b, subtitleFileFor(engine, db))
}

// wireBotCheck lets a plugin with sources.botcheck open a Source's bot
// check. With several, the first by id serves. Its pin is its id and the
// hosts it may reach, so a Source with a key needs the key typed again
// when the solver, or where it can send what it is handed, changes.
func wireBotCheck(b *hooks.Broker, reach func(id string) []string) {
	sources.BotCheck = func() *sources.BotCheckSolver {
		ids := b.Providers(hooks.Botcheck)
		if len(ids) == 0 {
			return nil
		}
		slices.Sort(ids)
		id := ids[0]
		return &sources.BotCheckSolver{Pin: sources.BotCheckPin(id, reach(id)), Solve: func(ctx context.Context, ask sources.BotCheckAsk) (sources.BotCheckAnswer, error) {
			in := hooks.BotcheckAsk{URL: ask.URL}
			if p := ask.Proxy; p != nil {
				pass, _ := p.User.Password()
				in.Proxy = &hooks.BotcheckProxy{URL: p.Scheme + "://" + p.Host, Username: p.User.Username(), Password: pass}
			}
			a, err := hooks.SolveBotcheck(ctx, b, id, in)
			if err != nil {
				return sources.BotCheckAnswer{}, err
			}
			out := sources.BotCheckAnswer{URL: a.URL, Status: a.Status, Body: a.Body, UserAgent: a.UserAgent}
			for _, k := range a.Cookies {
				out.Cookies = append(out.Cookies, sources.BotCheckCookie{Name: k.Name, Value: k.Value})
			}
			return out, nil
		}}
	}
}

// wireSubtitleSites lets a plugin with sources.subtitles serve a subtitle
// site (plugin://<id>). It is told where the video is only when the file
// sits in a folder the owner gave it, and never the full path.
func wireSubtitleSites(b *hooks.Broker, where func(ctx context.Context, plugin string, fileID int64) (hooks.SubtitleFile, error)) {
	sites.PluginSite = func(id string) sites.Site { return pluginSubtitleSite{b: b, id: id, where: where} }
}

// pluginSubtitleSite is a subtitle site a plugin serves through the Broker.
type pluginSubtitleSite struct {
	b     *hooks.Broker
	id    string
	where func(ctx context.Context, plugin string, fileID int64) (hooks.SubtitleFile, error)
}

// Search asks the plugin what it can make. Every offer is machine-made, and
// its link is the video's id, so the plugin can't point a make elsewhere.
func (p pluginSubtitleSite) Search(ctx context.Context, q sites.Query) ([]sites.Candidate, error) {
	file, err := p.where(ctx, p.id, q.FileID)
	if err != nil {
		return nil, err
	}
	ask := hooks.SubtitleAsk{Kind: q.Kind, Title: q.Title, Year: q.Year, Season: q.Season, Episode: q.Episode, IMDb: q.IMDb, TMDB: q.TMDB, TVDB: q.TVDB,
		ReleaseName: q.ReleaseName, Languages: q.Languages, SubtitleFile: file}
	offers, err := hooks.SearchSubtitles(ctx, p.b, p.id, ask)
	if err != nil {
		return nil, pluginSiteError(err)
	}
	link := strconv.FormatInt(q.FileID, 10)
	out := make([]sites.Candidate, 0, len(offers))
	for _, o := range offers {
		out = append(out, sites.Candidate{Key: o.Key, Language: o.Language, HearingImpaired: o.HearingImpaired, Forced: o.Forced,
			Format: o.Format, MachineMade: sites.Transcribed, Link: link})
	}
	return out, nil
}

// Download asks the plugin to make the offered subtitle; it may take an hour.
func (p pluginSubtitleSite) Download(ctx context.Context, c sites.Candidate) ([]byte, error) {
	fileID, err := strconv.ParseInt(c.Link, 10, 64)
	if err != nil || fileID <= 0 {
		return nil, errors.New("the subtitle's video is not known")
	}
	file, err := p.where(ctx, p.id, fileID)
	if err != nil {
		return nil, err
	}
	m, err := hooks.MakeSubtitle(ctx, p.b, p.id, hooks.SubtitleMake{Key: c.Key, Language: c.Language, SubtitleFile: file})
	if err != nil {
		return nil, pluginSiteError(err)
	}
	return m.Body, nil
}

// pluginSiteError marks a plugin that is not there (stopped, restarting or
// swamped) so a make Job tries again once it is back.
func pluginSiteError(err error) error {
	if errors.Is(err, hooks.ErrNoProvider) || errors.Is(err, hooks.ErrDropped) || errors.Is(err, hooks.ErrBusy) {
		return fmt.Errorf("%w (%v)", sites.ErrPluginGone, err)
	}
	return err
}

// subtitleFileFor says where a video is for plugin: the folder it was given
// that holds the file and the path inside it, links followed. Before the
// plugins have started it says the plugin is not running, so a plugin that
// polls early is never sent a video without its folder.
func subtitleFileFor(engine func() *plugins.Engine, db database.Querier) func(ctx context.Context, plugin string, fileID int64) (hooks.SubtitleFile, error) {
	return func(ctx context.Context, plugin string, fileID int64) (hooks.SubtitleFile, error) {
		f := hooks.SubtitleFile{FileID: fileID}
		e := engine()
		if e == nil {
			return f, fmt.Errorf("%w (the plugins are still starting)", sites.ErrPluginGone)
		}
		p, err := mediafiles.FilePath(ctx, db, fileID)
		if err != nil {
			return f, nil
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			return f, nil
		}
		if name, rel, ok := e.FolderOf(plugin, real); ok {
			f.Folder, f.Path = name, rel
		}
		return f, nil
	}
}

// pluginDownloader is the download client a plugin serves.
func pluginDownloader(reg *hooks.Registry) func(id string) downloads.Engine {
	return func(id string) downloads.Engine { return &hooks.PluginEngine{Broker: reg.Broker, Plugin: id} }
}

// pluginHookRoutes registers the hook routes with the app's own checks.
func (a *handlers) pluginHookRoutes(m *http.ServeMux) {
	reg := a.s.PluginHooks
	if reg == nil {
		reg = hooks.NewRegistry(hooks.NewBroker()) // no plugins: every hook answers that none provides it
	}
	h := &hooks.Handlers{Reg: reg, ValidScope: accounts.ValidPluginScope}
	if a.s.Auth != nil {
		h.Users = func(ctx context.Context) ([]hooks.User, error) {
			ps, err := a.s.Auth.PeopleNames(ctx)
			out := make([]hooks.User, 0, len(ps))
			for _, p := range ps {
				out = append(out, hooks.User{ID: p.ID, Name: p.Name})
			}
			return out, err
		}
	}
	h.StreamFile = func(ctx context.Context, plugin string, fileID int64) (string, error) {
		return streamPath(ctx, a.s.Plugins, a.s.Store.Read(), plugin, fileID)
	}
	if a.s.PluginStreamFile != nil {
		h.StreamFile = a.s.PluginStreamFile
	}
	if a.s.Library != nil {
		h.SetArtwork = func(ctx context.Context, titleID int64, kind string, pic []byte) error {
			return a.s.Library.SetPluginArtwork(ctx, titleID, kind, pic, webserver.Actor(ctx))
		}
	}
	if a.s.Alerts != nil {
		h.Notify = a.s.Alerts.SendPlugin
	}
	h.BridgeAllowed = func(plugin string, who webserver.Caller, r *http.Request) error {
		_, pattern := m.Handler(r)
		return accounts.PluginBridgeAllowed(accounts.PluginTokenScopes(plugin), who, pattern)
	}
	h.AsPlugin = func(plugin string, r *http.Request) (*http.Response, error) {
		scopes := accounts.PluginTokenScopes(plugin)
		if len(scopes) == 0 {
			return nil, errors.New("the plugin is not running")
		}
		who := webserver.Caller{Via: "plugin", PluginID: plugin, Name: "plugin " + plugin, Scopes: scopes}
		rec := httptest.NewRecorder()
		accounts.Gate(m).ServeHTTP(rec, r.WithContext(webserver.ContextWith(r.Context(), who)))
		return rec.Result(), nil
	}
	h.Routes(m)
}

// streamPath is a media file's path when it sits inside a folder the owner
// granted plugin.
func streamPath(ctx context.Context, e *plugins.Engine, db database.Querier, plugin string, fileID int64) (string, error) {
	if e == nil {
		return "", errors.New("plugins are not set up")
	}
	p, err := mediafiles.FilePath(ctx, db, fileID)
	if err != nil {
		return "", errors.New("no such file")
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real // a link out of the folder does not count as inside it
	} else {
		return "", errors.New("no such file")
	}
	if !insideAny(p, e.GrantedFolders(plugin)) {
		return "", errors.New("this file is not in a folder the plugin was given")
	}
	return p, nil
}

// insideAny says whether p is one of roots or inside one.
func insideAny(p string, roots []string) bool {
	p = filepath.Clean(p)
	for _, r := range roots {
		r = filepath.Clean(r)
		if rel, err := filepath.Rel(r, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

// pluginTools hands assistant tools from plugins to the AI route. A tool
// shows and runs only for a person whose own rights cover its scopes, and
// the plugin acts with those scopes only.
type pluginTools struct{ reg *hooks.Registry }

func (p pluginTools) covers(who webserver.Caller) func(string) bool {
	return func(s string) bool { return accounts.CallerCoversPluginScope(who, s) }
}

func (p pluginTools) List(who webserver.Caller) []map[string]any {
	var out []map[string]any
	for _, t := range p.reg.ToolsFor(p.covers(who)) {
		in := t.Input
		if in == nil {
			in = map[string]any{"type": "object"}
		}
		out = append(out, map[string]any{"name": t.Name, "description": hooks.UntrustedNote + " " + t.About, "inputSchema": in,
			"annotations": map[string]any{"readOnlyHint": false, "openWorldHint": true}})
	}
	return out
}

func (p pluginTools) Call(r *http.Request, who webserver.Caller, name string, args map[string]any) (any, error) {
	return p.reg.CallTool(r.Context(), name, args, p.covers(who), accounts.IssuePluginCallToken)
}
