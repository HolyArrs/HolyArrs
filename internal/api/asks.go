package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func (a *API) askRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/asks", a.listAsks)
	m.HandleFunc("POST /api/v1/asks", a.makeAsk)
	m.HandleFunc("GET /api/v1/asks/limit", a.askLimit)
	m.HandleFunc("GET /api/v1/asks/{id}", a.getAsk)
	m.HandleFunc("DELETE /api/v1/asks/{id}", a.withdrawAsk)
	m.HandleFunc("POST /api/v1/asks/{id}/approve", a.approveAsk)
	m.HandleFunc("POST /api/v1/asks/{id}/turn-down", a.turnDownAsk)
	m.HandleFunc("GET /api/v1/settings/asks", a.getAskSettings)
	m.HandleFunc("PUT /api/v1/settings/asks", a.putAskSettings)
	m.HandleFunc("GET /api/v1/ask-limits", a.listAskLimits)
	m.HandleFunc("PUT /api/v1/ask-limits/{id}", a.putAskLimit)
	m.HandleFunc("GET /api/v1/messages", a.listMessages)
	m.HandleFunc("POST /api/v1/messages/read", a.readMessages)
	m.HandleFunc("GET /api/v1/my/alerts", a.getMyAlerts)
	m.HandleFunc("PUT /api/v1/my/alerts", a.putMyAlerts)
	m.HandleFunc("GET /api/v1/settings/alerts", a.getAlertSettings)
	m.HandleFunc("PUT /api/v1/settings/alerts", a.putAlertSettings)
	if a.s.Secrets != nil {
		m.HandleFunc("GET /api/v1/alerts", a.listAlerts)
		m.HandleFunc("POST /api/v1/alerts", a.addAlert)
		m.HandleFunc("PUT /api/v1/alerts/{id}", a.editAlert)
		m.HandleFunc("DELETE /api/v1/alerts/{id}", a.deleteAlert)
		m.HandleFunc("POST /api/v1/alerts/{id}/test", a.testAlert)
	}
}

// person is the signed-in person as the Wishlist sees them. Without
// sign-in (tests) it is user 1 with every right.
func (a *API) person(r *http.Request) asks.Person {
	who, ok := auth.From(r.Context())
	if !ok {
		var name string
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT name FROM users WHERE id = 1").Scan(&name)
		return asks.Person{UserID: 1, Name: name, AutoApprove: true, NoLimit: true}
	}
	return asks.Person{UserID: who.UserID, Name: who.Name, AutoApprove: who.Has(auth.PermAutoApprove), NoLimit: who.Has(auth.PermApprove),
		MaxRating: a.kidMax(r)}
}

// mayApprove says whether the signed-in person decides asks.
func mayApprove(r *http.Request) bool {
	who, ok := auth.From(r.Context())
	return !ok || who.Has(auth.PermApprove)
}

func askError(w http.ResponseWriter, err error) {
	var in *asks.InLibraryError
	var le *asks.LimitError
	switch {
	case errors.As(err, &in):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": in.TitleID, "ready": in.Ready})
	case errors.As(err, &le):
		secs := int(time.Until(le.RetryAt).Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": err.Error(), "retry_at": le.RetryAt, "per_week": le.PerWeek, "kind": le.Kind})
	case errors.Is(err, asks.ErrNotOpen), errors.Is(err, asks.ErrAskedAgain):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, asks.ErrNotYours), errors.Is(err, asks.ErrAboveLimit):
		fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, asks.ErrNoteLength), errors.Is(err, asks.ErrPerWeek), errors.Is(err, asks.ErrEmail), errors.Is(err, asks.ErrDiscordID),
		errors.Is(err, asks.ErrNoSuchSeason), errors.Is(err, asks.ErrShowWhoAsked), errors.Is(err, asks.ErrNotAskedPart):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrNoSuchAlbum), errors.Is(err, library.ErrCopyChoice):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrAlbumNotIncluded), errors.Is(err, asks.ErrAlbumNotListedYet):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "no such ask")
	case errors.Is(err, library.ErrBadArtistID), errors.Is(err, library.ErrArtistNeverAdd), errors.Is(err, library.ErrNoMusicSource),
		errors.Is(err, library.ErrNoMusicFolder), errors.Is(err, library.ErrNotMusicFolder), errors.Is(err, library.ErrAlbumNotIncluded),
		errors.Is(err, metadata.ErrArtistNotFound), errors.Is(err, metadata.ErrMusicBusy), errors.Is(err, metadata.ErrMusicUnavailable):
		musicError(w, err)
	default:
		showError(w, err)
	}
}

func (a *API) askViews(r *http.Request, where string, args ...any) ([]asks.View, error) {
	vs, err := a.s.Asks.Views(r.Context(), where, args...)
	if err != nil {
		return nil, err
	}
	me := a.person(r).UserID
	// Artist asks show the picture route's artist picture; let it serve them.
	var names []nameKey
	for _, v := range vs {
		if v.Kind == "artist" && v.ArtistID != "" {
			names = append(names, nameKey{kind: "artist", mbid: strings.ToLower(v.ArtistID), name: v.Name})
		}
	}
	a.rememberMusicNames(names)
	for i := range vs {
		for _, p := range vs[i].People {
			vs[i].Mine = vs[i].Mine || p.ID == me
		}
	}
	s, err := a.seer(r.Context())
	if err != nil {
		return nil, err
	}
	for i := range vs {
		s.trim(&vs[i])
	}
	return vs, nil
}

// listAsks lists everyone's asks for people who approve, else only the
// person's own. ?mine=1 and ?state=<state> narrow the list.
func (a *API) listAsks(w http.ResponseWriter, r *http.Request) {
	where, args := []string{"1=1"}, []any{}
	if !mayApprove(r) || r.URL.Query().Get("mine") == "1" {
		where = append(where, "EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)")
		args = append(args, a.person(r).UserID)
	}
	if k := r.URL.Query().Get("kind"); k != "" {
		if k != "movie" && k != "show" && k != "artist" {
			fail(w, http.StatusBadRequest, "kind must be movie, show or artist")
			return
		}
		where = append(where, "a.kind = ?")
		args = append(args, k)
	}
	if st := r.URL.Query().Get("state"); st != "" {
		if asks.StateLabels[st] == "" {
			fail(w, http.StatusBadRequest, "state must be asked, approved, turned_down or available")
			return
		}
		where = append(where, "a.state = ?")
		args = append(args, st)
	}
	vs, err := a.askViews(r, strings.Join(where, " AND "), args...)
	if err != nil {
		internal(w, err)
		return
	}
	vs = vs[:min(len(vs), limit(r, 200))]
	writeJSON(w, http.StatusOK, map[string]any{"asks": vs})
}

func (a *API) askByID(w http.ResponseWriter, r *http.Request) (asks.View, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such ask")
		return asks.View{}, false
	}
	vs, err := a.askViews(r, "a.id = ?", id)
	if err != nil {
		internal(w, err)
		return asks.View{}, false
	}
	// Someone else's ask is not shown to people who do not approve.
	if len(vs) == 0 || (!vs[0].Mine && !mayApprove(r)) {
		fail(w, http.StatusNotFound, "no such ask")
		return asks.View{}, false
	}
	return vs[0], true
}

func (a *API) getAsk(w http.ResponseWriter, r *http.Request) {
	if v, ok := a.askByID(w, r); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

func (a *API) answerAsk(w http.ResponseWriter, r *http.Request, code int, id int64, extra map[string]any) {
	vs, err := a.askViews(r, "a.id = ?", id)
	if err != nil || len(vs) == 0 {
		internal(w, errors.Join(err, errors.New("ask vanished")))
		return
	}
	out := map[string]any{"ask": vs[0]}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, code, out)
}

func (a *API) makeAsk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`      // "tmdb:693134"; for a show also tvdb:, tvmaze:, imdb:; for an artist mbid:
		Kind    string `json:"kind"`    // movie (default), show or artist
		Seasons []int  `json:"seasons"` // a show's seasons; empty or missing: the whole show
		// FutureOnly asks a show for its future seasons only (no seasons list).
		FutureOnly bool     `json:"future_only"`
		Albums     []string `json:"albums"` // an artist's albums (MusicBrainz ids); empty or missing: the whole artist
		// Editions picks an edition per asked album: album id to release id.
		Editions map[string]string `json:"editions"`
		// Follow puts the title on the asker's calendar (default true).
		Follow *bool `json:"follow"`
	}
	if !decode(w, r, &body, askHelp) {
		return
	}
	if len(body.Editions) > 0 && body.Kind != "artist" {
		fail(w, http.StatusBadRequest, askHelp)
		return
	}
	// No one can ask for a title of a part that is off.
	askKind := body.Kind
	if askKind == "" {
		askKind = "movie"
	}
	if p, known := parts.ForTitleKind(askKind); known && !a.partIsOn(p) {
		writeJSON(w, http.StatusConflict, partOffBody(parts.Of(p)))
		return
	}
	src, num, ok := strings.Cut(strings.TrimSpace(body.ID), ":")
	var res asks.Result
	var err error
	switch body.Kind {
	case "", "movie":
		if !ok || src != "tmdb" || num == "" || len(num) > 12 || strings.Trim(num, "0123456789") != "" || len(body.Seasons) > 0 || len(body.Albums) > 0 || body.FutureOnly {
			fail(w, http.StatusBadRequest, askHelp)
			return
		}
		res, err = a.s.Asks.Ask(r.Context(), a.person(r), num)
	case "show":
		if !ok || num == "" || len(num) > 20 || (src != "tmdb" && src != "tvdb" && src != "tvmaze" && src != "imdb") || len(body.Seasons) > 200 || len(body.Albums) > 0 ||
			(body.FutureOnly && len(body.Seasons) > 0) {
			fail(w, http.StatusBadRequest, askHelp)
			return
		}
		if body.FutureOnly {
			res, err = a.s.Asks.AskShowFutureOnly(r.Context(), a.person(r), src+":"+num)
		} else {
			res, err = a.s.Asks.AskShow(r.Context(), a.person(r), src+":"+num, body.Seasons)
		}
	case "artist":
		if !ok || (src != "mbid" && src != "musicbrainz") || !metadata.ValidMBID(num) || len(body.Seasons) > 0 || len(body.Albums) > 500 || body.FutureOnly {
			fail(w, http.StatusBadRequest, askHelp)
			return
		}
		for _, al := range body.Albums {
			if !metadata.ValidMBID(strings.TrimSpace(al)) {
				fail(w, http.StatusBadRequest, askHelp)
				return
			}
		}
		ctx := r.Context()
		if !mayApprove(r) {
			ctx = metadata.MemberLookup(ctx) // behind the refresh and staff at MusicBrainz
		}
		ed := map[string]string{}
		for al, rel := range body.Editions {
			al, rel = strings.ToLower(strings.TrimSpace(al)), strings.ToLower(strings.TrimSpace(rel))
			if !metadata.ValidMBID(al) || !metadata.ValidMBID(rel) || !slices.ContainsFunc(body.Albums, func(x string) bool { return strings.EqualFold(strings.TrimSpace(x), al) }) {
				fail(w, http.StatusBadRequest, askHelp)
				return
			}
			ed[al] = rel
		}
		res, err = a.s.Asks.AskArtistEditions(ctx, a.person(r), "mbid:"+num, body.Albums, ed)
	default:
		fail(w, http.StatusBadRequest, askHelp)
		return
	}
	follow := body.Follow == nil || *body.Follow
	if err != nil {
		// Asking for a title already in the Library still follows it.
		var in *asks.InLibraryError
		if follow && errors.As(err, &in) && in.TitleID != 0 {
			if ferr := a.s.Asks.FollowAdded(r.Context(), a.person(r), in.TitleID); ferr != nil {
				log.Printf("api: follow on ask: %v", ferr)
			}
		}
		askError(w, err)
		return
	}
	if err := a.s.Asks.FollowAsk(r.Context(), a.person(r), res.Ask.ID, follow); err != nil {
		log.Printf("api: follow on ask %d: %v", res.Ask.ID, err)
	}
	code := http.StatusCreated
	if res.Joined || res.Already {
		code = http.StatusOK
	}
	a.answerAsk(w, r, code, res.Ask.ID, map[string]any{"joined": res.Joined, "already": res.Already})
}

const askHelp = `send {"id": "tmdb:<number>"} for a movie, {"kind": "show", "id": "tmdb:<n>" (or tvdb:, tvmaze:, imdb:), "seasons": [1, 2]} for a show (no seasons: the whole show; "future_only": true instead of seasons: the future seasons only), or {"kind": "artist", "id": "mbid:<MusicBrainz artist id>", "albums": ["<MusicBrainz album id>"], "editions": {"<album id>": "<release id>"}} for music (no albums: the whole artist; editions optional); any ask may add "follow": false to leave the title off your calendar`

func (a *API) askLimit(w http.ResponseWriter, r *http.Request) {
	l, err := a.s.Asks.LimitOf(r.Context(), a.person(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (a *API) withdrawAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := a.askByID(w, r)
	if !ok {
		return
	}
	if err := a.s.Asks.Withdraw(r.Context(), v.ID, a.person(r), mayApprove(r)); err != nil {
		askError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) approveAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := a.askByID(w, r)
	if !ok {
		return
	}
	// An ask of a part that is off waits: approving it would add a title there.
	if p, known := parts.ForTitleKind(v.Kind); known && !a.partIsOn(p) {
		writeJSON(w, http.StatusConflict, partOffBody(parts.Of(p)))
		return
	}
	var opt asks.ApproveOptions
	if r.ContentLength != 0 && !decode(w, r, &opt, `send {} or any of library_folder_id, pick_rules_id, audio_languages, seasons, albums, note`) {
		return
	}
	if _, err := a.s.Asks.Approve(r.Context(), v.ID, a.person(r), opt); err != nil {
		askError(w, err)
		return
	}
	a.answerAsk(w, r, http.StatusOK, v.ID, nil)
}

func (a *API) turnDownAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := a.askByID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body, `send {"note": "why"}`) {
		return
	}
	if _, err := a.s.Asks.TurnDown(r.Context(), v.ID, a.person(r), body.Note); err != nil {
		askError(w, err)
		return
	}
	a.answerAsk(w, r, http.StatusOK, v.ID, nil)
}

func (a *API) getAskSettings(w http.ResponseWriter, r *http.Request) {
	s, err := asks.ReadSettings(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) putAskSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved values.
	s, err := asks.ReadSettings(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &s, "send any of movies_per_week, shows_per_week, music_per_week, library_folder_id, pick_rules_id, search_on_approve, show_library_folder_id, show_pick_rules_id, music_library_folder_id, music_pick_rules_id, show_who_asked") {
		return
	}
	if err := asks.SaveSettings(r.Context(), a.s.Store, s, actor(r)); err != nil {
		askError(w, err)
		return
	}
	a.getAskSettings(w, r)
}

// askLimitView is one person's weekly limits, one per kind.
type askLimitView struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	asks.PersonLimit
	// Used is how many of their own asks of each kind count this week.
	Used    map[string]int `json:"used"`
	NoLimit bool           `json:"no_limit"` // they may approve, so no limit applies
}

func (a *API) listAskLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT id, name, role, perms FROM users ORDER BY name`)
	if err != nil {
		internal(w, err)
		return
	}
	type row struct {
		v     askLimitView
		perms string
	}
	var list []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.v.UserID, &x.v.Name, &x.v.Role, &x.perms); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		list = append(list, x)
	}
	rows.Close()
	out := []askLimitView{}
	for _, x := range list {
		p := asks.Person{UserID: x.v.UserID, NoLimit: x.v.Role == auth.RoleOwner || strings.Contains(x.perms, `"`+string(auth.PermApprove)+`"`)}
		l, err := a.s.Asks.LimitOf(ctx, p)
		if err != nil {
			internal(w, err)
			return
		}
		if x.v.Movies, x.v.Shows, x.v.Music, err = asks.PersonLimits(ctx, a.s.Store.Read(), x.v.UserID); err != nil {
			internal(w, err)
			return
		}
		x.v.Used = map[string]int{asks.KindMovies: l.Movies.Used, asks.KindShows: l.Shows.Used, asks.KindMusic: l.Music.Used}
		x.v.NoLimit = p.NoLimit
		out = append(out, x.v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"limits": out})
}

func (a *API) putAskLimit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	var body struct {
		asks.PersonLimit
		PerWeek *int `json:"per_week"` // older form: the same limit for every kind
	}
	if !decode(w, r, &body, `send {"movies_per_week": <0 to 1000, 0 is no limit, null for the default>, "shows_per_week": ..., "music_per_week": ...}`) {
		return
	}
	l := body.PersonLimit
	if body.PerWeek != nil && l.Movies == nil && l.Shows == nil && l.Music == nil {
		l = asks.PersonLimit{Movies: body.PerWeek, Shows: body.PerWeek, Music: body.PerWeek}
	}
	if err := asks.SetPersonLimit(r.Context(), a.s.Store, id, l); errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "no such user")
		return
	} else if err != nil {
		askError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": id, "movies_per_week": l.Movies, "shows_per_week": l.Shows, "music_per_week": l.Music})
}

func (a *API) listMessages(w http.ResponseWriter, r *http.Request) {
	ms, unread, err := asks.Messages(r.Context(), a.s.Store.Read(), a.person(r).UserID, limit(r, 30))
	if err != nil {
		internal(w, err)
		return
	}
	out := map[string]any{"messages": ms, "unread": unread}
	if mayApprove(r) {
		var n int
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT count(*) FROM asks WHERE state = 'asked'").Scan(&n)
		out["waiting"] = n
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) readMessages(w http.ResponseWriter, r *http.Request) {
	if err := asks.MarkRead(r.Context(), a.s.Store, a.person(r).UserID); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getMyAlerts(w http.ResponseWriter, r *http.Request) {
	t, err := asks.ReadTargets(r.Context(), a.s.Store.Read(), a.person(r).UserID)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *API) putMyAlerts(w http.ResponseWriter, r *http.Request) {
	var t asks.Targets
	if !decode(w, r, &t, `send {"email": "...", "discord_id": "...", "new_episodes": true}`) {
		return
	}
	t.Email, t.DiscordID = strings.TrimSpace(t.Email), strings.TrimSpace(t.DiscordID)
	if err := asks.SaveTargets(r.Context(), a.s.Store, a.person(r).UserID, t); err != nil {
		askError(w, err)
		return
	}
	t, err := asks.ReadTargets(r.Context(), a.s.Store.Read(), a.person(r).UserID)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// AlertView is an Alert as the API shows it: secrets are masked.
type AlertView struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Enabled      bool       `json:"enabled"`
	Moments      []string   `json:"moments"`
	URL          string     `json:"url,omitempty"`
	SMTPHost     string     `json:"smtp_host,omitempty"`
	SMTPPort     int        `json:"smtp_port,omitempty"`
	SMTPSecurity string     `json:"smtp_security,omitempty"`
	SMTPFrom     string     `json:"smtp_from,omitempty"`
	SMTPUser     string     `json:"smtp_user,omitempty"`
	Password     string     `json:"smtp_pass,omitempty"`
	StaffTo      string     `json:"staff_to,omitempty"`
	Secret       string     `json:"secret,omitempty"`
	WebhookAuth  string     `json:"webhook_auth,omitempty"`
	AllowLocal   bool       `json:"allow_local_network"`
	LastSentAt   *time.Time `json:"last_sent_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

func alertView(x domain.Alert) AlertView {
	v := AlertView{ID: x.ID, Name: x.Name, Kind: x.Kind, Enabled: x.Enabled, Moments: x.Moments, AllowLocal: x.AllowLocal, LastSentAt: x.LastSentAt, LastError: x.LastError}
	if v.Moments == nil {
		v.Moments = []string{}
	}
	if x.Kind == "discord" || x.Kind == "webhook" {
		if x.URLSecretID != nil {
			v.URL = secrets.Masked
		}
		if x.Kind == "webhook" {
			v.WebhookAuth = x.WebhookAuth
			if x.PasswordSecretID != nil {
				v.Secret = secrets.Masked
			}
		}
		return v
	}
	v.SMTPHost, v.SMTPPort, v.SMTPSecurity, v.SMTPFrom, v.SMTPUser, v.StaffTo = x.SMTPHost, x.SMTPPort, x.SMTPSecurity, x.SMTPFrom, x.SMTPUser, x.StaffTo
	if x.PasswordSecretID != nil {
		v.Password = secrets.Masked
	}
	return v
}

func alertError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, alerts.ErrNameUsed):
		fail(w, http.StatusConflict, err.Error())
	case alerts.IsInputError(err):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "no such Alert")
	default:
		internal(w, err)
	}
}

func (a *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	xs, err := store.Select[domain.Alert](r.Context(), a.s.Store.Read(), "alerts", "1=1 ORDER BY name")
	if err != nil {
		internal(w, err)
		return
	}
	out := []AlertView{}
	for _, x := range xs {
		out = append(out, alertView(x))
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out, "moments": alerts.Moments})
}

const alertHelp = `send name, kind (discord, email or webhook), moments, and for Discord url; for email smtp_host, smtp_port, smtp_security, smtp_from, smtp_user, smtp_pass, staff_to; for a webhook url, secret and webhook_auth (sign, bearer or none); allow_local_network lets it reach the local network`

// readAlert reads an Alert body; the masked placeholder keeps the stored secret.
func readAlert(w http.ResponseWriter, r *http.Request) (alerts.Input, bool) {
	var in alerts.Input
	if !decode(w, r, &in, alertHelp) {
		return in, false
	}
	if in.URL == secrets.Masked {
		in.URL = ""
	}
	if in.Password == secrets.Masked {
		in.Password = ""
	}
	if in.Secret == secrets.Masked {
		in.Secret = ""
	}
	return in, true
}

func (a *API) addAlert(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	in, ok := readAlert(w, r)
	if !ok {
		return
	}
	x, err := alerts.Add(r.Context(), a.s.Store, a.s.Secrets, in, actor(r))
	if err != nil {
		alertError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, alertView(x))
}

func alertID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such Alert")
		return 0, false
	}
	return id, true
}

func (a *API) editAlert(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := alertID(w, r)
	if !ok {
		return
	}
	in, ok := readAlert(w, r)
	if !ok {
		return
	}
	x, err := alerts.Change(r.Context(), a.s.Store, a.s.Secrets, id, in, actor(r))
	if err != nil {
		alertError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alertView(x))
}

func (a *API) deleteAlert(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := alertID(w, r)
	if !ok {
		return
	}
	if err := alerts.Remove(r.Context(), a.s.Store, id); err != nil {
		alertError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testAlert sends a test message through one Alert. A Discord Alert posts
// it; an email Alert sends it to the staff address, else to the person
// testing (their own email from My account).
func (a *API) testAlert(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := alertID(w, r)
	if !ok {
		return
	}
	var x domain.Alert
	if err := store.Get(r.Context(), a.s.Store.Read(), "alerts", id, &x); err != nil {
		alertError(w, err)
		return
	}
	m := alerts.Message{Moment: alerts.MomentWaiting, Subject: "HolyArrs test", Text: "This is a test from HolyArrs. If you can read it, the Alert " + x.Name + " works."}
	if x.Kind == "webhook" {
		m.Moment = alerts.MomentTest
	}
	if x.Kind == "email" && strings.TrimSpace(x.StaffTo) == "" {
		t, _ := asks.ReadTargets(r.Context(), a.s.Store.Read(), a.person(r).UserID)
		if t.Email == "" {
			fail(w, http.StatusBadRequest, "add a staff address to this Alert, or your own email in My account, to send a test")
			return
		}
		m.Moment, m.People = alerts.MomentReady, []alerts.Person{{Name: a.person(r).Name, Email: t.Email}}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := a.s.Alerts.TestOne(ctx, x, m); err != nil {
		// The reason (without links or secrets) is logged and kept on the
		// Alert as its last result; the answer stays the same every time.
		fail(w, http.StatusBadGateway, "the Discord, mail or webhook server refused the test")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// askBrief is an open ask shown next to a looked-up movie.
type askBrief struct {
	ID         int64  `json:"id"`
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
	Mine       bool   `json:"mine"`
	People     int    `json:"people"`
	Seasons    []int  `json:"seasons,omitempty"` // a show ask's seasons; empty: the whole show
	// Albums are an artist ask's albums; empty: the whole artist.
	Albums []domain.AskAlbum `json:"albums,omitempty"`
}

// openAsk finds the open ask for a TMDB id, if any.
func (a *API) openAsk(ctx context.Context, tmdbID string, me int64) (*askBrief, error) {
	var b askBrief
	err := a.s.Store.Read().QueryRowContext(ctx, `SELECT a.id, a.state, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'movie' AND a.tmdb_id = ? AND a.state IN ('asked', 'approved')`,
		me, tmdbID).Scan(&b.ID, &b.State, &b.Mine, &b.People)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.StateLabel = asks.StateLabels[b.State]
	return &b, nil
}

// openShowAsk finds the oldest open ask for a show with any of ids, if any.
func (a *API) openShowAsk(ctx context.Context, ids map[string]string, me int64) (*askBrief, error) {
	var or []string
	var args []any
	args = append(args, me)
	for _, k := range []string{"tmdb", "tvdb", "tvmaze", "imdb"} {
		if ids[k] != "" {
			or = append(or, "json_extract(a.source_ids, '$."+k+"') = ?")
			args = append(args, ids[k])
		}
	}
	if len(or) == 0 {
		return nil, nil
	}
	var b askBrief
	var seasons string
	err := a.s.Store.Read().QueryRowContext(ctx, `SELECT a.id, a.state, a.seasons, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'show' AND a.state IN ('asked', 'approved') AND (`+strings.Join(or, " OR ")+`)
		ORDER BY a.id LIMIT 1`, args...).Scan(&b.ID, &b.State, &seasons, &b.Mine, &b.People)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.StateLabel = asks.StateLabels[b.State]
	b.Seasons = []int{}
	_ = json.Unmarshal([]byte(seasons), &b.Seasons)
	return &b, nil
}

// titleAsks are the asks that led to a Title: who asked and who decided.
// People who do not approve see only the asks they are part of.
func (a *API) titleAsks(ctx context.Context, titleID int64) ([]asks.View, error) {
	who, ok := auth.From(ctx)
	if !ok || who.Has(auth.PermApprove) {
		return a.s.Asks.Views(ctx, "a.title_id = ?", titleID)
	}
	vs, err := a.s.Asks.Views(ctx, "a.title_id = ? AND EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)", titleID, who.UserID)
	s, serr := a.seer(ctx)
	for i := range vs {
		vs[i].Mine = true
		s.trim(&vs[i])
	}
	if err == nil {
		err = serr
	}
	return vs, err
}

// openArtistAsks lists every open ask for an artist, oldest first (an
// artist can have several, for different albums).
func (a *API) openArtistAsks(ctx context.Context, mbid string, me int64) ([]askBrief, error) {
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT a.id, a.state, a.albums, EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?),
		(SELECT count(*) FROM ask_people p WHERE p.ask_id = a.id) FROM asks a WHERE a.kind = 'artist' AND a.state IN ('asked', 'approved')
		AND json_extract(a.source_ids, '$.musicbrainz') = ? ORDER BY a.id`, me, mbid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []askBrief
	for rows.Next() {
		var b askBrief
		var albums string
		if err := rows.Scan(&b.ID, &b.State, &albums, &b.Mine, &b.People); err != nil {
			return nil, err
		}
		b.StateLabel = asks.StateLabels[b.State]
		b.Albums = []domain.AskAlbum{}
		_ = json.Unmarshal([]byte(albums), &b.Albums)
		out = append(out, b)
	}
	return out, rows.Err()
}
