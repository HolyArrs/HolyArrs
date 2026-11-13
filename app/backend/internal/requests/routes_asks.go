package requests

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// AskHandlers are the ask routes: asking, approving, Request rules,
// weekly limits, messages and the person's alert targets.
type AskHandlers struct {
	Svc   *Service
	Store *database.Store
	// Person is the signed-in person as Requests sees them.
	Person func(r *http.Request) Person
	// Actor is the signed-in person (the owner without sign-in).
	Actor func(r *http.Request) domain.Actor
	// PartOff answers 409 and says true when the part of a title kind is off.
	PartOff func(w http.ResponseWriter, kind string) bool
	// SeenArtists lets the picture route serve the artists of listed asks.
	SeenArtists func(vs []View)
	// MusicError and ShowError answer the music and show refusals.
	MusicError, ShowError func(w http.ResponseWriter, err error)
}

// Routes registers the ask routes.
func (h *AskHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/asks", h.listAsks)
	m.HandleFunc("POST /api/v1/asks", h.makeAsk)
	m.HandleFunc("GET /api/v1/asks/limit", h.askLimit)
	m.HandleFunc("GET /api/v1/asks/{id}", h.getAsk)
	m.HandleFunc("GET /api/v1/asks/{id}/parts", h.askParts)
	m.HandleFunc("DELETE /api/v1/asks/{id}", h.withdrawAsk)
	m.HandleFunc("POST /api/v1/asks/{id}/approve", h.approveAsk)
	m.HandleFunc("POST /api/v1/asks/{id}/turn-down", h.turnDownAsk)
	m.HandleFunc("GET /api/v1/settings/asks", h.getAskSettings)
	m.HandleFunc("PUT /api/v1/settings/asks", h.putAskSettings)
	m.HandleFunc("GET /api/v1/ask-limits", h.listAskLimits)
	m.HandleFunc("PUT /api/v1/ask-limits/{id}", h.putAskLimit)
	m.HandleFunc("GET /api/v1/messages", h.listMessages)
	m.HandleFunc("POST /api/v1/messages/read", h.readMessages)
	m.HandleFunc("GET /api/v1/my/alerts", h.getMyAlerts)
	m.HandleFunc("PUT /api/v1/my/alerts", h.putMyAlerts)
}

// mayApprove says whether the signed-in person decides asks.
func mayApprove(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermApprove)
}

func (h *AskHandlers) askError(w http.ResponseWriter, err error) {
	var in *InLibraryError
	var le *LimitError
	switch {
	case errors.As(err, &in):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": in.TitleID, "ready": in.Ready})
	case errors.As(err, &le):
		secs := int(time.Until(le.RetryAt).Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		webserver.WriteJSON(w, http.StatusTooManyRequests, map[string]any{"error": err.Error(), "retry_at": le.RetryAt, "per_week": le.PerWeek, "kind": le.Kind})
	case errors.Is(err, ErrNotAddedSlow):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrNotOpen), errors.Is(err, ErrAskedAgain), errors.Is(err, ErrQualityTaken):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrQualityNotOffered), errors.Is(err, ErrQualityName), errors.Is(err, ErrQualityAllHidden):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotYours), errors.Is(err, ErrAboveLimit):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNoteLength), errors.Is(err, ErrPerWeek), errors.Is(err, ErrEmail), errors.Is(err, ErrDiscordID),
		errors.Is(err, ErrNoSuchSeason), errors.Is(err, ErrBadEpisode), errors.Is(err, ErrNotAlbumTrack), errors.Is(err, ErrNotAskedPart), errors.Is(err, ErrNoFourKRules):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNoSuchAlbum), errors.Is(err, library.ErrCopyChoice):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAlbumNotIncluded), errors.Is(err, ErrAlbumNotListedYet):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such ask")
	case errors.Is(err, library.ErrBadArtistID), errors.Is(err, library.ErrArtistNeverAdd), errors.Is(err, library.ErrNoMusicSource),
		errors.Is(err, library.ErrNoMusicFolder), errors.Is(err, library.ErrNotMusicFolder), errors.Is(err, library.ErrAlbumNotIncluded),
		errors.Is(err, metadata.ErrArtistNotFound), errors.Is(err, metadata.ErrMusicBusy), errors.Is(err, metadata.ErrMusicUnavailable):
		h.MusicError(w, err)
	default:
		h.ShowError(w, err)
	}
}

func (h *AskHandlers) askViews(r *http.Request, where string, args ...any) ([]View, error) {
	vs, err := h.Svc.Views(r.Context(), where, args...)
	if err != nil {
		return nil, err
	}
	me := h.Person(r).UserID
	// Artist asks show the picture route's artist picture; let it serve them.
	if h.SeenArtists != nil {
		h.SeenArtists(vs)
	}
	for i := range vs {
		for _, p := range vs[i].People {
			vs[i].Mine = vs[i].Mine || p.ID == me
		}
	}
	// Where each approved ask is now: the search's own words for
	// why one waits are for people who approve.
	h.Svc.AddSteps(r.Context(), vs, mayApprove(r))
	// Who else declined the same title is for people who approve.
	// The people who asked see who decided by display name only, never
	// the sign-in name.
	if mayApprove(r) {
		h.Svc.AddAlsoDeclined(r.Context(), vs)
	} else {
		HideDeciderNames(vs)
	}
	return vs, nil
}

// listAsks lists everyone's asks for people who approve, else only the
// person's own. ?mine=1 and ?state=<state> narrow the list.
func (h *AskHandlers) listAsks(w http.ResponseWriter, r *http.Request) {
	where, args := []string{"1=1"}, []any{}
	if !mayApprove(r) || r.URL.Query().Get("mine") == "1" {
		where = append(where, askMineSQL)
		args = append(args, h.Person(r).UserID)
	}
	if k := r.URL.Query().Get("kind"); k != "" {
		if k != "movie" && k != "show" && k != "artist" {
			webserver.Fail(w, http.StatusBadRequest, "kind must be movie, show or artist")
			return
		}
		where = append(where, "a.kind = ?")
		args = append(args, k)
	}
	if st := r.URL.Query().Get("state"); st != "" {
		if StateLabels[st] == "" {
			webserver.Fail(w, http.StatusBadRequest, "state must be asked, approved, turned_down or available")
			return
		}
		where = append(where, "a.state = ?")
		args = append(args, st)
	}
	vs, err := h.askViews(r, strings.Join(where, " AND "), args...)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	vs = vs[:min(len(vs), askLimitParam(r, 200))]
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"asks": vs})
}

func (h *AskHandlers) askByID(w http.ResponseWriter, r *http.Request) (View, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such ask")
		return View{}, false
	}
	vs, err := h.askViews(r, "a.id = ?", id)
	if err != nil {
		webserver.Internal(w, "api", err)
		return View{}, false
	}
	// Someone else's ask is not shown to people who do not approve.
	if len(vs) == 0 || (!vs[0].Mine && !mayApprove(r)) {
		webserver.Fail(w, http.StatusNotFound, "no such ask")
		return View{}, false
	}
	return vs[0], true
}

func (h *AskHandlers) getAsk(w http.ResponseWriter, r *http.Request) {
	if v, ok := h.askByID(w, r); ok {
		webserver.WriteJSON(w, http.StatusOK, v)
	}
}

func (h *AskHandlers) answerAsk(w http.ResponseWriter, r *http.Request, code int, id int64, extra map[string]any) {
	vs, err := h.askViews(r, "a.id = ?", id)
	if err != nil || len(vs) == 0 {
		webserver.Internal(w, "api", errors.Join(err, errors.New("ask vanished")))
		return
	}
	out := map[string]any{"ask": vs[0]}
	for k, v := range extra {
		out[k] = v
	}
	webserver.WriteJSON(w, code, out)
}

func (h *AskHandlers) makeAsk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`      // "tmdb:693134"; for a show also tvdb:, tvmaze:, imdb:; for an artist mbid:
		Kind    string `json:"kind"`    // movie (default), show or artist
		Seasons []int  `json:"seasons"` // a show's seasons; empty or missing: the whole show
		// Episodes asks for single episodes of other seasons: [{"season": 2, "episodes": [1, 2, 5]}].
		Episodes []domain.AskEpisodes `json:"episodes"`
		// FutureOnly asks a show for its future seasons only (no seasons list).
		FutureOnly bool     `json:"future_only"`
		Albums     []string `json:"albums"` // an artist's albums (MusicBrainz ids); empty or missing: the whole artist
		// Editions picks an edition per asked album: album id to release id.
		Editions map[string]string `json:"editions"`
		// Follow puts the title on the asker's calendar (default true).
		Follow *bool `json:"follow"`
		// FourK asks a movie or show for a 4K copy (approved apart from a plain ask).
		FourK bool `json:"four_k"`
		// Quality picks a line of what people can ask for by id (standard,
		// 4k; GET /asks/limit lists the ones the person may use). Empty:
		// the 4K line with four_k, else the person's first line.
		Quality string `json:"quality"`
	}
	if !webserver.Decode(w, r, &body, askHelp) {
		return
	}
	if (len(body.Editions) > 0 && body.Kind != "artist") || ((body.FourK || body.Quality != "") && body.Kind == "artist") ||
		(body.FourK && body.Quality != "" && body.Quality != QualityFourK) || len(body.Quality) > 40 {
		webserver.Fail(w, http.StatusBadRequest, askHelp)
		return
	}
	if body.FourK {
		body.Quality = QualityFourK
	}
	// No one can ask for a title of a part that is off.
	askKind := body.Kind
	if askKind == "" {
		askKind = "movie"
	}
	if h.PartOff != nil && h.PartOff(w, askKind) {
		return
	}
	src, num, ok := strings.Cut(strings.TrimSpace(body.ID), ":")
	var res Result
	var err error
	switch body.Kind {
	case "", "movie":
		if !ok || src != "tmdb" || num == "" || len(num) > 12 || strings.Trim(num, "0123456789") != "" || len(body.Seasons) > 0 || len(body.Episodes) > 0 || len(body.Albums) > 0 || body.FutureOnly {
			webserver.Fail(w, http.StatusBadRequest, askHelp)
			return
		}
		res, err = h.Svc.AskQuality(r.Context(), h.Person(r), num, body.Quality)
	case "show":
		if !ok || num == "" || len(num) > 20 || (src != "tmdb" && src != "tvdb" && src != "tvmaze" && src != "imdb") || len(body.Seasons) > 200 || len(body.Albums) > 0 ||
			len(body.Episodes) > 200 || (body.FutureOnly && (len(body.Seasons) > 0 || len(body.Episodes) > 0)) || tooManyEpisodes(body.Episodes) {
			webserver.Fail(w, http.StatusBadRequest, askHelp)
			return
		}
		res, err = h.Svc.AskShowQuality(r.Context(), h.Person(r), src+":"+num, body.Seasons, body.Episodes, body.FutureOnly, body.Quality)
	case "artist":
		if !ok || (src != "mbid" && src != "musicbrainz") || !metadata.ValidMBID(num) || len(body.Seasons) > 0 || len(body.Episodes) > 0 || len(body.Albums) > 500 || body.FutureOnly {
			webserver.Fail(w, http.StatusBadRequest, askHelp)
			return
		}
		for _, al := range body.Albums {
			if !metadata.ValidMBID(strings.TrimSpace(al)) {
				webserver.Fail(w, http.StatusBadRequest, askHelp)
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
				webserver.Fail(w, http.StatusBadRequest, askHelp)
				return
			}
			ed[al] = rel
		}
		res, err = h.Svc.AskArtistEditions(ctx, h.Person(r), "mbid:"+num, body.Albums, ed)
	default:
		webserver.Fail(w, http.StatusBadRequest, askHelp)
		return
	}
	follow := body.Follow == nil || *body.Follow
	if err != nil {
		// Asking for a title already in the Library still follows it.
		var in *InLibraryError
		if follow && errors.As(err, &in) && in.TitleID != 0 {
			if ferr := h.Svc.FollowAdded(r.Context(), h.Person(r), in.TitleID); ferr != nil {
				log.Printf("api: follow on ask: %v", ferr)
			}
		}
		h.askError(w, err)
		return
	}
	if err := h.Svc.FollowAsk(r.Context(), h.Person(r), res.Ask.ID, follow); err != nil {
		log.Printf("api: follow on ask %d: %v", res.Ask.ID, err)
	}
	code := http.StatusCreated
	if res.Joined || res.Already {
		code = http.StatusOK
	}
	h.answerAsk(w, r, code, res.Ask.ID, map[string]any{"joined": res.Joined, "already": res.Already})
}

const askHelp = `send {"id": "tmdb:<number>"} for a movie, {"kind": "show", "id": "tmdb:<n>" (or tvdb:, tvmaze:, imdb:), "seasons": [1, 2]} for a show (no seasons: the whole show; "episodes": [{"season": 2, "episodes": [1, 2, 5]}] adds single episodes of other seasons; "future_only": true instead of seasons: the future seasons only), or {"kind": "artist", "id": "mbid:<MusicBrainz artist id>", "albums": ["<MusicBrainz album id>"], "editions": {"<album id>": "<release id>"}} for music (no albums: the whole artist; editions optional); any ask may add "follow": false to leave the title off your calendar; a movie or show ask may add "four_k": true to ask for a 4K copy`

func (h *AskHandlers) askLimit(w http.ResponseWriter, r *http.Request) {
	l, err := h.Svc.LimitOf(r.Context(), h.Person(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// Qualities are the lines of what people can ask for that this person
	// may use, per kind, in order (the Quality row on the Ask sheets; one
	// line or none hides the row). FourK says which kinds offer the 4K line.
	out := struct {
		Limit
		FourK struct {
			Movie bool `json:"movie"`
			Show  bool `json:"show"`
		} `json:"four_k"`
		Qualities struct {
			Movie []QualityChoice `json:"movie"`
			Show  []QualityChoice `json:"show"`
		} `json:"qualities"`
	}{Limit: l}
	set, err := ReadSettings(r.Context(), h.Svc.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, kind := range []string{"movie", "show"} {
		lines, err := qualityLines(r.Context(), h.Svc.Store.Read(), set, kind)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		mine := choicesOf(offered(lines, h.Person(r)))
		_, fourK := lineByID(offered(lines, h.Person(r)), QualityFourK)
		if kind == "show" {
			out.Qualities.Show, out.FourK.Show = mine, fourK
		} else {
			out.Qualities.Movie, out.FourK.Movie = mine, fourK
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *AskHandlers) withdrawAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := h.askByID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Withdraw(r.Context(), v.ID, h.Person(r), mayApprove(r)); err != nil {
		h.askError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// askParts answers an ask's stored lists (season episodes, album songs),
// stored when it was asked for, so the Requests panel opens them at once.
func (h *AskHandlers) askParts(w http.ResponseWriter, r *http.Request) {
	v, ok := h.askByID(w, r)
	if !ok {
		return
	}
	p, err := h.Svc.AskParts(r.Context(), v.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	webserver.WriteJSON(w, http.StatusOK, p)
}

func (h *AskHandlers) approveAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := h.askByID(w, r)
	if !ok {
		return
	}
	// An ask of a part that is off waits: approving it would add a title there.
	if h.PartOff != nil && h.PartOff(w, v.Kind) {
		return
	}
	var opt ApproveOptions
	if r.ContentLength != 0 && !webserver.Decode(w, r, &opt, `send {} or any of library_folder_id, pick_rules_id, quality, audio_languages, seasons, episodes, albums, note`) {
		return
	}
	_, said, err := h.Svc.ApproveSaying(r.Context(), v.ID, h.Person(r), opt)
	if err != nil {
		h.askError(w, err)
		return
	}
	// Notice tells the approver their quality was not used (the copy was there).
	var extra map[string]any
	if said != "" {
		extra = map[string]any{"notice": said}
	}
	h.answerAsk(w, r, http.StatusOK, v.ID, extra)
}

func (h *AskHandlers) turnDownAsk(w http.ResponseWriter, r *http.Request) {
	v, ok := h.askByID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &body, `send {"note": "why"}`) {
		return
	}
	if _, err := h.Svc.TurnDown(r.Context(), v.ID, h.Person(r), body.Note); err != nil {
		h.askError(w, err)
		return
	}
	h.answerAsk(w, r, http.StatusOK, v.ID, nil)
}

func (h *AskHandlers) getAskSettings(w http.ResponseWriter, r *http.Request) {
	s, err := ReadSettings(r.Context(), h.Store.Read())
	if err == nil {
		s.Qualities, err = resolveQualities(r.Context(), h.Store.Read(), s)
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, s)
}

func (h *AskHandlers) putAskSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved values.
	s, err := ReadSettings(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &s, "send any of movies_per_week, shows_per_week, music_per_week, library_folder_id, pick_rules_id, search_on_approve, show_library_folder_id, show_pick_rules_id, music_library_folder_id, music_pick_rules_id, qualities") {
		return
	}
	if err := SaveSettings(r.Context(), h.Store, s, h.Actor(r)); err != nil {
		h.askError(w, err)
		return
	}
	h.getAskSettings(w, r)
}

// AskLimitView is one person's weekly limits, one per kind.
type AskLimitView struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	PersonLimit
	// Used is how many of their own asks of each kind count this week.
	Used    map[string]int `json:"used"`
	NoLimit bool           `json:"no_limit"` // they may approve, so no limit applies
}

func (h *AskHandlers) listAskLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.Store.Read().QueryContext(ctx, askLimitUsersSQL)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	type row struct {
		v     AskLimitView
		perms string
	}
	var list []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.v.UserID, &x.v.Name, &x.v.Role, &x.perms); err != nil {
			rows.Close()
			webserver.Internal(w, "api", err)
			return
		}
		list = append(list, x)
	}
	rows.Close()
	out := []AskLimitView{}
	for _, x := range list {
		p := Person{UserID: x.v.UserID, NoLimit: x.v.Role == webserver.RoleOwner || strings.Contains(x.perms, `"`+string(webserver.PermApprove)+`"`)}
		l, err := h.Svc.LimitOf(ctx, p)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if x.v.Movies, x.v.Shows, x.v.Music, err = PersonLimits(ctx, h.Store.Read(), x.v.UserID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		x.v.Used = map[string]int{KindMovies: l.Movies.Used, KindShows: l.Shows.Used, KindMusic: l.Music.Used}
		x.v.NoLimit = p.NoLimit
		out = append(out, x.v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"limits": out})
}

func (h *AskHandlers) putAskLimit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such user")
		return
	}
	var body struct {
		PersonLimit
		PerWeek *int `json:"per_week"` // older form: the same limit for every kind
	}
	if !webserver.Decode(w, r, &body, `send {"movies_per_week": <0 to 1000, 0 is no limit, null for the default>, "shows_per_week": ..., "music_per_week": ...}`) {
		return
	}
	l := body.PersonLimit
	if body.PerWeek != nil && l.Movies == nil && l.Shows == nil && l.Music == nil {
		l = PersonLimit{Movies: body.PerWeek, Shows: body.PerWeek, Music: body.PerWeek}
	}
	if err := SetPersonLimit(r.Context(), h.Store, id, l); errors.Is(err, database.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no such user")
		return
	} else if err != nil {
		h.askError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"user_id": id, "movies_per_week": l.Movies, "shows_per_week": l.Shows, "music_per_week": l.Music})
}

func (h *AskHandlers) listMessages(w http.ResponseWriter, r *http.Request) {
	ms, unread, err := Messages(r.Context(), h.Store.Read(), h.Person(r).UserID, askLimitParam(r, 30))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := map[string]any{"messages": ms, "unread": unread}
	if mayApprove(r) {
		var n int
		_ = h.Store.Read().QueryRowContext(r.Context(), askWaitingSQL).Scan(&n)
		out["waiting"] = n
	}
	if MayManageReports(r) {
		out["reports_open"], _ = OpenReports(r.Context(), h.Store.Read())
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *AskHandlers) readMessages(w http.ResponseWriter, r *http.Request) {
	if err := MarkRead(r.Context(), h.Store, h.Person(r).UserID); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AskHandlers) getMyAlerts(w http.ResponseWriter, r *http.Request) {
	t, err := ReadTargets(r.Context(), h.Store.Read(), h.Person(r).UserID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, t)
}

func (h *AskHandlers) putMyAlerts(w http.ResponseWriter, r *http.Request) {
	var t Targets
	if !webserver.Decode(w, r, &t, `send {"email": "...", "discord_id": "...", "new_episodes": true}`) {
		return
	}
	t.Email, t.DiscordID = strings.TrimSpace(t.Email), strings.TrimSpace(t.DiscordID)
	if err := SaveTargets(r.Context(), h.Store, h.Person(r).UserID, t); err != nil {
		h.askError(w, err)
		return
	}
	t, err := ReadTargets(r.Context(), h.Store.Read(), h.Person(r).UserID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, t)
}

// tooManyEpisodes caps single episodes per season in an ask.
func tooManyEpisodes(eps []domain.AskEpisodes) bool {
	for _, e := range eps {
		if len(e.Episodes) > 500 {
			return true
		}
	}
	return false
}

// askLimitParam reads ?limit= (1 to 500), else def.
func askLimitParam(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}
