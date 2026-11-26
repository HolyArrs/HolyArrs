package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

func (b *builder) people(s *Snapshot) {
	b.cover("users", "settings")
	for i, u := range s.Users {
		from := b.from("users", i)
		name := clip(u.Name, 64)
		if name == "" {
			name = clip(u.Email, 64)
		}
		if name == "" {
			b.not(GroupPeople, from, "(no name)", "a person needs a name")
			continue
		}
		key := stablePersonKey(s.Kind, u)
		legacyKey := personKey(s.Kind, u.SourceID)
		b.personRefs[legacyKey] = key
		pp := &PersonPlan{App: string(s.Kind), SourceID: u.SourceID, StableIdentity: key != legacyKey, Email: clip(u.Email, 200), Role: "member", Perms: []string{}}
		has := func(n ...string) bool { return hasField(u.Permissions, n...) }
		// Roles cap at member: the owner confirms an admin, or that the old
		// app's owner is them, person by person on the move screen.
		pp.WasOwner = u.Owner
		pp.WasAdmin = !u.Owner && (u.Admin || has("ADMIN"))
		choice := b.choices[key]
		owner := pp.WasOwner && choice == PersonOwner
		switch {
		case owner:
			pp.Role = "owner"
		case pp.WasAdmin && choice == PersonAdmin:
			pp.Role = "admin"
		}
		if pp.WasAdmin && pp.Role == "admin" {
			b.not(GroupPeople, from+".permissions", name+": ADMIN", "the confirmed HolyArrs admin role keeps account administration; other source-admin capabilities are not granted automatically")
		}
		if choice == PersonMerge || choice == PersonSkip {
			pp.Choice = choice
		}
		add := func(p string, ok bool) {
			if ok {
				pp.Perms = append(pp.Perms, p)
			}
		}
		add("ask", has("REQUEST") || pp.Role != "member")
		add("auto_approve", has("AUTO_APPROVE"))
		add("auto_approve_four_k", has("AUTO_APPROVE_4K") || has("AUTO_APPROVE_4K_MOVIE") && has("AUTO_APPROVE_4K_TV"))
		add("approve", has("MANAGE_REQUESTS"))
		add("settings", has("MANAGE_SETTINGS") && pp.Role == "admin")
		if has("MANAGE_SETTINGS") && pp.Role != "admin" {
			b.not(GroupPeople, from+".permissions", name+": MANAGE_SETTINGS", "settings access requires a confirmed HolyArrs admin role, so it is not granted")
		}
		var dropped []string
		for _, p := range u.Permissions {
			switch p {
			case "ADMIN", "REQUEST", "AUTO_APPROVE", "MANAGE_REQUESTS", "MANAGE_SETTINGS", "AUTO_APPROVE_4K":
				continue
			case "AUTO_APPROVE_4K_MOVIE", "AUTO_APPROVE_4K_TV":
				// Both halves together became the 4K permission above.
				if has("AUTO_APPROVE_4K_MOVIE") && has("AUTO_APPROVE_4K_TV") {
					continue
				}
			}
			dropped = append(dropped, clip(p, 40))
		}
		if len(dropped) > 0 {
			b.not(GroupPeople, from+".permissions", name+": "+strings.Join(dropped, ", "), "no HolyArrs permission matches these")
		}
		pp.MoviesPerWeek = weekly(u.MovieLimit, u.MovieDays)
		pp.ShowsPerWeek = weekly(u.ShowLimit, u.ShowDays)
		if u.MovieLimit < 0 || u.MovieLimit > 1000 {
			b.not(GroupPeople, from+".movie_quota", name, "the per-person quota is outside HolyArrs' supported range of 1 to 1000, so it is not approximated")
		} else if u.MovieLimitSet && u.MovieLimit == 0 {
			b.not(GroupPeople, from+".movie_quota", name, "an explicit zero per-person quota has no established source meaning, so it is left at the HolyArrs default")
		} else if u.MovieLimit > 0 && pp.MoviesPerWeek == nil {
			b.not(GroupPeople, from+".movie_quota", name, "only an exact seven-day quota can be imported without changing its access window")
		}
		if u.ShowLimit < 0 || u.ShowLimit > 1000 {
			b.not(GroupPeople, from+".show_quota", name, "the per-person quota is outside HolyArrs' supported range of 1 to 1000, so it is not approximated")
		} else if u.ShowLimitSet && u.ShowLimit == 0 {
			b.not(GroupPeople, from+".show_quota", name, "an explicit zero per-person quota has no established source meaning, so it is left at the HolyArrs default")
		} else if u.ShowLimit > 0 && pp.ShowsPerWeek == nil {
			b.not(GroupPeople, from+".show_quota", name, "only an exact seven-day quota can be imported without changing its access window")
		}
		// A Plex, Jellyfin or Emby person keeps that sign-in.
		if kind := strings.ToLower(u.Kind); key != legacyKey && signInWay(kind) != "" {
			pp.SignIn, pp.SignInID, pp.SignInName = kind, clip(u.ExternalID, 100), clip(u.ExtName, 200)
		}
		detail := "Comes over turned off as a " + pp.Role + ". Invite them to sign in; their password stays behind."
		needs := "an invite"
		if way := signInWay(pp.SignIn); way != "" {
			detail = "Comes over as a " + pp.Role + ", linked to their " + way + " account. Their first sign-in with it turns them on; no password comes over."
			needs = ""
		}
		why := ""
		switch {
		case owner:
			detail = "This is you. Their requests become yours."
			why = "you confirmed they are you (the owner in " + string(s.Kind) + ")"
		case pp.WasOwner:
			why = "Owner in " + kindName(s.Kind) + ". Turn on This is me if it's you."
		case pp.WasAdmin && pp.Role == "admin":
			why = "an admin in " + string(s.Kind) + "; you confirmed the admin role"
		case pp.WasAdmin:
			why = "an admin in " + string(s.Kind) + "; comes over as a member unless you confirm the admin role"
		}
		if pp.MoviesPerWeek != nil {
			detail += fmt.Sprintf("; %d movie asks a week", *pp.MoviesPerWeek)
		}
		if pp.ShowsPerWeek != nil {
			detail += fmt.Sprintf("; %d show asks a week", *pp.ShowsPerWeek)
		}
		b.planned[key] = true
		var legacy []string
		if legacyKey != key {
			legacy = []string{legacyKey}
		}
		if owner {
			needs = ""
		}
		b.add(GroupPeople, Item{Key: key, LegacyKeys: legacy, Name: name, From: from, Detail: detail, Why: why, Person: pp, Needs: needs})
	}
	recognized := map[string]bool{}
	movies := defaultWeekly(s.Settings, "movie")
	shows := defaultWeekly(s.Settings, "show")
	accountDefault := func(kind string, planned *int) {
		limitKey := "default_" + kind + "_limit"
		daysKey := "default_" + kind + "_days"
		limitRaw, hasLimit := s.Settings[limitKey]
		_, hasDays := s.Settings[daysKey]
		for _, key := range []string{limitKey, daysKey} {
			if _, ok := s.Settings[key]; ok {
				recognized[key] = true
			}
		}
		from := b.from("settings", -1)
		if planned != nil {
			b.p.Mapped = append(b.p.Mapped, from+"."+limitKey)
			if *planned > 0 && hasDays {
				b.p.Mapped = append(b.p.Mapped, from+"."+daysKey)
			} else if hasDays {
				b.not(GroupRequests, from+"."+daysKey, kind+" default", "the quota is unlimited, so its old access-window length is not used")
			}
			return
		}
		if hasLimit {
			n := int(num(limitRaw))
			reason := "only an exact seven-day quota can be imported without changing its access window"
			if n < 0 || n > 1000 {
				reason = "the default quota is outside HolyArrs' supported range of 0 to 1000, so it is not approximated"
			}
			b.not(GroupRequests, from+"."+limitKey, kind+" default", reason)
		}
		if hasDays {
			reason := "the access-window length has no matching default quota to import"
			if hasLimit {
				reason = "the default quota pair cannot be imported without changing its meaning"
			}
			b.not(GroupRequests, from+"."+daysKey, kind+" default", reason)
		}
	}
	accountDefault("movie", movies)
	accountDefault("show", shows)
	if movies != nil || shows != nil {
		as := &AskSettingsPlan{MoviesPerWeek: movies, ShowsPerWeek: shows}
		b.add(GroupRequests, Item{Key: "wishlist_defaults", Name: "Request defaults", From: b.from("settings", -1),
			Detail: "default weekly limits", AskSettings: as})
	}
	// Radarr's availability delay is a Release gate setting (titleList).
	recognized["availability_delay"] = s.Kind == Radarr
	// The "unmonitor deleted" switch is a file rule (fileRules).
	recognized["unkeep_deleted"] = s.Kind == Radarr || s.Kind == Sonarr
	keys := make([]string, 0, len(s.Settings))
	for k := range s.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if recognized[k] {
			continue
		}
		b.not(GroupPeople, b.from("settings", -1)+"."+k, k, "app-wide settings of the old app are not imported; set the Request defaults under Settings > Request rules")
	}
}

// signInWays are the sign-in kinds a person can keep, by product name.
var signInWays = map[string]string{"plex": "Plex", "jellyfin": "Jellyfin", "emby": "Emby"}

// signInWay names a sign-in kind a person can keep, or "".
func signInWay(kind string) string { return signInWays[kind] }

func (b *builder) asks(s *Snapshot) {
	b.cover("requests")
	for i, r := range s.Requests {
		from := b.from("requests", i)
		var kind, ref string
		ids := map[string]string{}
		for _, src := range []string{"tmdb", "tvdb", "imdb"} {
			if v := clip(r.IDs[src], 20); v != "" && (src == "imdb" || isDigits(v)) {
				ids[src] = v
			}
		}
		switch r.Kind {
		case "movie":
			kind = "movie"
			if ids["tmdb"] != "" {
				ref = "tmdb:" + ids["tmdb"]
			}
		case "show":
			kind = "show"
			for _, src := range []string{"tvdb", "tmdb"} {
				if ids[src] != "" && ref == "" {
					ref = src + ":" + ids[src]
				}
			}
		}
		if ref == "" {
			b.not(GroupRequests, from, "ask "+strconv.FormatInt(r.SourceID, 10), "no id HolyArrs can find it by")
			continue
		}
		name := b.titles[kind+":"+ref]
		if name == "" {
			// In a fixed order, so the same read always gives the same name.
			for _, src := range []string{"tmdb", "tvdb", "imdb"} {
				if n := b.titles[kind+":ids:"+ids[src]]; ids[src] != "" && n != "" {
					name = n
					break
				}
			}
		}
		if name == "" {
			name = requests.PlaceholderName(ref) // the real name is looked up when it is imported
		}
		byKey := b.personRefs[personKey(s.Kind, r.By)]
		ap := &AskPlan{App: string(s.Kind), Kind: kind, Ref: ref, IDs: ids, Approved: r.State == "approved", Done: r.Done,
			By: byKey, ByName: clip(r.ByName, 64), AskedAt: clip(r.Created, 40), DecidedAt: clip(r.Updated, 40)}
		if ap.AskedAt != "" {
			if asked, ok := sourceTime(ap.AskedAt); !ok {
				b.not(GroupRequests, from+".created", ap.AskedAt, "the source timestamp format is invalid, so the import cannot preserve it")
				ap.AskedAt = ""
			} else if s.ReadAt.IsZero() {
				b.not(GroupRequests, from+".created", ap.AskedAt, "this kept read has no capture time, so the source timestamp cannot be verified")
				ap.AskedAt = ""
			} else if asked.After(s.ReadAt) {
				b.not(GroupRequests, from+".created", ap.AskedAt, "the source timestamp is after this kept read, so the import cannot preserve it")
				ap.AskedAt = ""
			}
		} else {
			b.not(GroupRequests, from+".created", "(missing)", "the source has no request timestamp, so the import cannot preserve it")
		}
		if ap.Approved && ap.DecidedAt == "" {
			b.not(GroupRequests, from+".updated", "(missing)", "the source has no decision timestamp, so the import cannot preserve it")
		} else if ap.DecidedAt != "" {
			if decided, ok := sourceTime(ap.DecidedAt); !ok {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "the source decision timestamp format is invalid, so the import cannot preserve it")
				ap.DecidedAt = ""
			} else if s.ReadAt.IsZero() {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "this kept read has no capture time, so the source decision timestamp cannot be verified")
				ap.DecidedAt = ""
			} else if decided.After(s.ReadAt) {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "the source decision timestamp is after this kept read, so the import cannot preserve it")
				ap.DecidedAt = ""
			}
		}
		if !b.planned[ap.By] {
			ap.By = ""
		}
		if k := b.personRefs[personKey(s.Kind, r.DecidedBy)]; r.DecidedBy != 0 && b.planned[k] {
			ap.Decider = k
		}
		for _, n := range r.Seasons {
			if n >= 0 && n < 10000 && len(ap.Seasons) < 200 {
				ap.Seasons = append(ap.Seasons, n)
			}
		}
		ap.FourK = r.FourK
		state := "waiting"
		if ap.Approved {
			state = "approved"
		}
		if ap.Done {
			state = "done"
		}
		by := ap.ByName
		if by == "" {
			by = "someone"
		}
		b.add(GroupRequests, Item{Key: fmt.Sprintf("ask:%s:%d", s.Kind, r.SourceID), Name: name, From: from,
			Detail: state + ", asked by " + by, Ask: ap})
	}
}

func (b *builder) alerts(s *Snapshot) {
	b.cover("notifications")
	for i, n := range s.Notifications {
		from := b.from("notifications", i)
		var ap *AlertPlan
		switch strings.ToLower(n.Kind) {
		case "discord":
			ap = &AlertPlan{Kind: "discord"}
		case "webhook":
			ap = &AlertPlan{Kind: "webhook"}
		case "email":
			port, _ := setNum(n.Settings, "port")
			sec := "starttls"
			switch strings.ToLower(setStr(n.Settings, "useEncryption", "requireEncryption")) {
			case "2", "never", "false":
				sec = "none"
			case "1", "always":
				sec = "tls"
			}
			if port <= 0 || port > 65535 {
				port = 587
			}
			to := setStr(n.Settings, "to")
			if v := list(anyKey(n.Settings, "to")); len(v) > 0 {
				to = str(v[0])
			}
			ap = &AlertPlan{Kind: "email", SMTPHost: clip(setStr(n.Settings, "server", "host"), 200), SMTPPort: int(port), SMTPSecurity: sec,
				SMTPFrom: clip(setStr(n.Settings, "from"), 200), SMTPUser: clip(setStr(n.Settings, "username", "user"), 200), StaffTo: clip(to, 200)}
		case "telegram":
			// The bot token is a secret and never comes over; the chat does
			// when it is one HolyArrs can write to.
			ap = &AlertPlan{Kind: "telegram"}
			if chat := strings.TrimSpace(setStr(n.Settings, "chatId")); alerts.CheckTelegramChat(chat) == nil {
				ap.ChatID = chat
			}
		case "ntfy":
			// The topics are tokens in disguise, so the read drops them; the
			// server comes as its scheme and host (linkSafe).
			ap = &AlertPlan{Kind: "ntfy"}
			ap.ServerURL, ap.AllowLocal = alertServer(n.Settings, "serverUrl")
		case "gotify":
			ap = &AlertPlan{Kind: "gotify"}
			ap.ServerURL, ap.AllowLocal = alertServer(n.Settings, "server")
		case "pushover":
			ap = &AlertPlan{Kind: "pushover"}
		}
		if ap == nil {
			b.not(GroupAlerts, from, n.Name, "HolyArrs Alerts go to Discord, a webhook, email, Telegram, ntfy, Gotify or Pushover only")
			continue
		}
		needs := map[string]string{"discord": "its Discord link", "webhook": "its address", "email": "the mail password",
			"telegram": "its bot token", "ntfy": "its server and topic", "gotify": "its server and app token",
			"pushover": "its user key and app token"}[ap.Kind]
		switch {
		case ap.Kind == "telegram" && ap.ChatID == "":
			needs = "its bot token and chat"
		case ap.Kind == "ntfy" && ap.ServerURL != "":
			needs = "its topic"
		case ap.Kind == "gotify" && ap.ServerURL != "":
			needs = "its app token"
		}
		why := "Alerts speak for every moment at first; pick the moments again"
		if ap.Moments = alertMoments(n.Settings); ap.Moments != nil {
			why = "Alerts speak for the same moments as in the old app"
		}
		b.addTagged(s.Kind, GroupAlerts, Item{Key: "alert:" + strings.ToLower(clip(n.Name, 80)), Name: clip(n.Name, 80), From: from, Alert: ap,
			Detail: ap.Kind + "; added off until you type " + needs, Needs: needs, Why: why}, n.Tags)
		if left := alertLeft(ap.Kind, n.Settings); len(left) > 0 {
			b.not(GroupAlerts, from, clip(n.Name, 80), fmt.Sprintf("HolyArrs has no place for: %s", strings.Join(left, "; ")))
		}
	}
}

// alertServer is the ntfy or Gotify server the read kept (scheme and host)
// when HolyArrs may send to it, and whether it is on the local network.
func alertServer(set map[string]any, key string) (string, bool) {
	root, _ := serverRoot(setStr(set, key))
	if root == "" || metadataHost(root) || alerts.CheckServerURL(root, true) != nil {
		return "", false
	}
	return root, alerts.CheckServerURL(root, false) != nil
}

// alertMomentOf maps an old app's on-switch to a HolyArrs moment.
var alertMomentOf = map[string]string{"onGrab": alerts.MomentGrab, "onDownload": alerts.MomentImport, "onUpgrade": alerts.MomentImport,
	"onReleaseImport": alerts.MomentImport, "onRename": alerts.MomentRename, "onSeriesAdd": alerts.MomentAdd, "onMovieAdded": alerts.MomentAdd,
	"onArtistAdd": alerts.MomentAdd, "onSeriesDelete": alerts.MomentDelete, "onMovieDelete": alerts.MomentDelete, "onArtistDelete": alerts.MomentDelete,
	"onHealthIssue": alerts.MomentHealth, "onHealthRestored": alerts.MomentHealth, "onManualInteractionRequired": alerts.MomentDecision}

// alertMoments are the moments the old app's on-switches turn on, in the
// order screens show them; nil when the read has none of the switches or
// all are off (the Alert then speaks for every moment, as before).
func alertMoments(set map[string]any) []string {
	on := map[string]bool{}
	for k, m := range alertMomentOf {
		if boolean(anyKey(set, "link."+k)) {
			on[m] = true
		}
	}
	var out []string
	for _, m := range alerts.Moments {
		if on[m] {
			out = append(out, m)
		}
	}
	return out
}

// alertLeft names an old Alert's settings HolyArrs has no place for, only
// those set to something other than their usual value.
func alertLeft(kind string, set map[string]any) []string {
	var out []string
	other := func(key string, usual int64) bool {
		n, ok := setNum(set, key)
		return ok && n != usual
	}
	filled := func(key string) bool {
		v := anyKey(set, key)
		return len(list(v)) > 0 || list(v) == nil && strings.TrimSpace(str(v)) != "" && strings.TrimSpace(str(v)) != "[]"
	}
	switch kind {
	case "telegram":
		if other("topicId", 0) {
			out = append(out, "the Telegram topic")
		}
		if boolean(anyKey(set, "sendSilently")) {
			out = append(out, "sending silently")
		}
	case "ntfy":
		if other("priority", 3) {
			out = append(out, "the ntfy priority")
		}
		if filled("tags") {
			out = append(out, "the ntfy tags")
		}
		if u := clip(strings.TrimSpace(setStr(set, "userName", "username")), 80); u != "" {
			out = append(out, fmt.Sprintf("the user name %s (HolyArrs signs in to ntfy with an access token)", u))
		}
	case "gotify":
		if other("priority", 5) {
			out = append(out, "the Gotify priority")
		}
	case "pushover":
		if filled("devices") {
			out = append(out, "the Pushover devices")
		}
		if other("priority", 0) {
			out = append(out, "the Pushover priority")
		}
		if filled("sound") {
			out = append(out, "the Pushover sound")
		}
	}
	return out
}

// leftBehind lists the parts of a Snapshot the move never brings.
func (b *builder) leftBehind(s *Snapshot) {
	b.cover("tags", "import_lists", "metadata", "app_links", "app_profiles")
	for i, t := range s.Tags {
		b.not(GroupOther, b.from("tags", i), t.Label, "the import does not bring tags over")
	}
	for i, l := range s.ImportLists {
		if (s.Kind == Radarr || s.Kind == Sonarr) && b.outsideList(s.Kind, i, l) {
			continue
		}
		kind := strings.ToLower(l.Kind)
		var why string
		switch {
		case s.Kind == Lidarr:
			why = "HolyArrs Outside lists add movies only for now; this music list stays in Lidarr"
		// An MDBList list, by its own kind or a list link to mdblist.com, is
		// added again by hand: its link is not brought over yet. It comes before
		// the other-instance case, since Radarr's custom list kind is RadarrListImport.
		case strings.HasPrefix(kind, "mdblist") || strings.Contains(strings.ToLower(setStr(l.Settings, "url", "listUrl")), "mdblist.com"): // not imdblist
			why = "HolyArrs reads MDBList lists; add it again under Settings > Outside lists"
		case strings.Contains(kind, "radarr") || strings.Contains(kind, "sonarr"):
			why = "HolyArrs is one app; it does not copy from another instance"
		// Best effort: the TMDB list kind and the Trakt list kinds other than
		// popular lists can be added again here; the other kinds cannot.
		case kind == "tmdblistimport" || (strings.Contains(kind, "trakt") && !strings.Contains(kind, "popular")):
			why = "HolyArrs reads TMDB lists and public Trakt lists; add it again under Settings > Outside lists"
			if strings.Contains(kind, "trakt") {
				why += " (a private list needs Sign in with Trakt there)"
			}
		case s.Kind == Sonarr:
			why = "HolyArrs reads TMDB lists and public Trakt lists only; this list stays in Sonarr"
		default:
			why = "HolyArrs reads TMDB lists and public Trakt lists only; this list stays in Radarr"
		}
		if len(l.NeedsReentry) > 0 {
			why += "; its sign-in does not come over"
		}
		b.not(GroupOther, b.from("import_lists", i), l.Name, why)
	}
	for i, m := range s.Metadata {
		b.not(GroupOther, b.from("metadata", i), m.Name, "HolyArrs writes no metadata files for media players")
	}
	for i, l := range s.AppLinks {
		b.not(GroupOther, b.from("app_links", i), l.Name, "HolyArrs is one app; it needs no links between its parts")
	}
	for i := range s.AppProfiles {
		b.not(GroupOther, b.from("app_profiles", i), "sync profile", "HolyArrs is one app; nothing is synced between apps")
	}
}

// listSlug turns a Trakt list's name as Radarr keeps it into its address
// part: lower case, every run of other characters one dash.
func listSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// freeListName is name, or name with " (radarr)", " (radarr 2)" and so on
// (" (sonarr)" for a Sonarr list) when taken says that one is in use.
func freeListName(name string, app Kind, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	for n := 1; n < 100; n++ {
		end := " (" + string(app) + ")"
		if n > 1 {
			end = fmt.Sprintf(" (%s %d)", app, n)
		}
		if c := clip(name, 80-len(end)) + end; !taken(c) {
			return c
		}
	}
	return name
}

// syncHours reads a list's refresh gap ("12:00:00" or "1.00:00:00") as
// Outside list hours, 6 to 168; 12 when it cannot be read.
func syncHours(v any) int {
	s := strings.TrimSpace(str(v))
	days := 0
	if d, rest, ok := strings.Cut(s, "."); ok && strings.Count(rest, ":") == 2 {
		days, _ = strconv.Atoi(d)
		s = rest
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 12
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || days < 0 || h < 0 || m < 0 {
		return 12
	}
	hours := days*24 + h
	if m > 0 {
		hours++
	}
	return min(max(hours, 6), 168)
}

// outsideList plans one Radarr or Sonarr list as a paused Outside list: a
// TMDB list (Radarr only), a Trakt list or a Trakt watchlist
// (requests.NormalListRef checks its reference). A Sonarr list adds shows.
// It says false for every other kind, which leftBehind lists.
func (b *builder) outsideList(app Kind, i int, l Provider) bool {
	from := b.from("import_lists", i)
	set := l.Settings
	name := clip(l.Name, 80)
	titleKind := libraryKind(app)
	var kind, ref string
	switch strings.ToLower(l.Kind) {
	case "tmdblistimport":
		if app != Radarr {
			return false
		}
		kind, ref = requests.ListTMDB, str(anyKey(set, "listId"))
	case "traktlistimport":
		kind, ref = requests.ListTrakt, setStr(set, "username", "authUser")+"/"+listSlug(str(anyKey(set, "listname")))
	case "traktuserimport":
		switch str(anyKey(set, "traktListType")) {
		case "0", "userWatchList", "":
			kind, ref = requests.ListTrakt, setStr(set, "username", "authUser")+"/watchlist"
		default:
			why := "HolyArrs reads Trakt lists and watchlists; a watched or collection list stays in Radarr"
			if app == Sonarr {
				why = "HolyArrs reads Trakt lists and watchlists; a watched or collection list stays in Sonarr"
			}
			b.not(GroupLists, from, l.Name, why)
			return true
		}
	default:
		return false
	}
	norm, err := requests.NormalListRef(kind, ref)
	if err != nil {
		why := "its TMDB list number did not come over; add it again under Settings > Outside lists"
		if kind == requests.ListTrakt {
			why = "its Trakt user or list name did not come over; add it again under Settings > Outside lists"
		}
		b.not(GroupLists, from, l.Name, why)
		return true
	}
	folder := oldPath(str(anyKey(set, "link.rootFolderPath")))
	lib := ""
	for _, f := range b.folders {
		if f.Kind == titleKind && f.OldPath == folder && folder != "" {
			lib = f.Path
		}
	}
	if lib == "" {
		b.not(GroupLists, from, l.Name, "its library folder "+clip(folder, 200)+" did not come over")
		return true
	}
	if name == "" {
		name = kind + " " + norm
	}
	name = freeListName(name, app, func(n string) bool { return b.listNames[strings.ToLower(n)] })
	b.listNames[strings.ToLower(name)] = true
	op := &OutsideListPlan{Kind: kind, Ref: norm, Library: lib, KeepUpToDate: true,
		SearchOnAdd:    boolean(anyKey(set, "link.searchOnAdd")) || boolean(anyKey(set, "link.searchForMissingEpisodes")),
		Preset:         b.presets[b.presetKey(app, num(anyKey(set, "link.qualityProfileId")))],
		SyncEveryHours: syncHours(anyKey(set, "link.minRefreshInterval"))}
	if app == Sonarr {
		op.TitleKind = "show"
		b.showListSeasons(from, l, op)
	} else {
		switch strings.ToLower(str(anyKey(set, "link.monitor"))) {
		case "none", "2":
			op.KeepUpToDate = false
		case "movieandcollection", "1":
			b.not(GroupLists, from+".monitor", l.Name+": its movie series", "lists add only the movies on the list, not the rest of their movie series; add those by hand")
		}
		if v := str(anyKey(set, "link.minimumAvailability")); v != "" {
			op.ReleaseGate = releaseGates[strings.ToLower(v)]
		}
	}
	// Radarr reads a list only while it is on, and adds its movies only
	// while its automatic add is on too. Sonarr has only the automatic add.
	on := l.Enabled
	if v := anyKey(set, "link.enabled"); v != nil {
		on = boolean(v)
	}
	was := "was off there"
	switch auto := anyKey(set, "link.enableAuto"); {
	case on && auto != nil && !boolean(auto):
		was = "was on there but added no movies by itself"
	case on:
		was = "was on there"
	}
	detail := fmt.Sprintf("%s %s into %s; added paused (%s), every %d hours", map[string]string{requests.ListTMDB: "TMDB list", requests.ListTrakt: "Trakt list"}[kind], norm, lib, was, op.SyncEveryHours)
	if op.ReleaseGate != "" {
		detail += "; release gate: " + gateWords[op.ReleaseGate]
	}
	if op.TitleKind == "show" {
		detail += "; seasons: " + op.Seasons + ", new seasons: " + op.NewSeasons
	}
	if !op.KeepUpToDate {
		detail += "; Keep up to date off"
	}
	if kind == requests.ListTrakt {
		// The old app read it signed in; here a private list needs Sign in
		// with Trakt, a public one only the key.
		detail += "; needs your Trakt key, and Sign in with Trakt if the list is private"
	}
	key := "outside_list:" + kind + ":" + strings.ToLower(norm)
	if op.TitleKind == "show" {
		// The same Trakt watchlist can feed both apps: one list for its
		// movies, one for its shows.
		key = "outside_list:" + kind + ":show:" + strings.ToLower(norm)
	}
	b.add(GroupLists, Item{Key: key, Name: name, From: from, Detail: detail,
		Why: "added paused, like Sources and Downloaders: check its folder and Pick rules, then turn it on", OutsideList: op})
	return true
}

// sonarrMonitor maps Sonarr's list monitor choice (a word, or the backup's
// number) to the Outside list seasons choice.
var sonarrMonitor = map[string]string{
	"all": library.SeasonsAll, "1": library.SeasonsAll, "missing": library.SeasonsAll, "3": library.SeasonsAll,
	"existing": library.SeasonsAll, "4": library.SeasonsAll, "recent": library.SeasonsAll, "9": library.SeasonsAll,
	"monitorspecials": library.SeasonsAll, "10": library.SeasonsAll, "unmonitorspecials": library.SeasonsAll, "11": library.SeasonsAll,
	"future": library.SeasonsFuture, "2": library.SeasonsFuture,
	"firstseason": requests.SeasonsFirst, "5": requests.SeasonsFirst, "pilot": requests.SeasonsFirst, "8": requests.SeasonsFirst,
	"lastseason": library.SeasonsLatest, "6": library.SeasonsLatest, "latestseason": library.SeasonsLatest, "7": library.SeasonsLatest,
	"none": library.SeasonsNone, "12": library.SeasonsNone, "skip": library.SeasonsNone, "13": library.SeasonsNone,
}

// showListSeasons sets a Sonarr list's seasons and new seasons on op. Every
// Sonarr monitor choice has a match here but the pilot, which keeps the first
// season. The series type is left to the show: HolyArrs tells anime itself.
func (b *builder) showListSeasons(from string, l Provider, op *OutsideListPlan) {
	set := l.Settings
	mon := strings.ToLower(str(anyKey(set, "link.shouldMonitor")))
	op.Seasons = sonarrMonitor[mon]
	switch {
	case op.Seasons == "":
		op.Seasons = library.SeasonsAll
	case mon == "pilot" || mon == "8":
		b.not(GroupLists, from+".monitor", l.Name+": its seasons", "HolyArrs cannot keep only the first episode; the list keeps the first season")
	case op.Seasons == library.SeasonsNone:
		op.KeepUpToDate = false
	}
	op.NewSeasons = "all"
	switch strings.ToLower(str(anyKey(set, "link.monitorNewItems"))) {
	case "none", "1":
		op.NewSeasons = "none"
	}
	if op.Seasons == library.SeasonsFuture {
		op.NewSeasons = "all"
	}
}

// metadataHost says an address names a cloud metadata address (the list
// NotMetadata in core/outbound refuses), which no Source or Downloader may
// reach.
func metadataHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLinkLocalMulticast() || outbound.MetadataIP(ip)
	}
	return h == "metadata" || strings.HasPrefix(h, "metadata.") || strings.HasPrefix(h, "instance-data")
}

// PlanHash names what a move would do: a move starts only from the plan
// its dry run showed, and another one is refused (ErrPlanChanged). It
// covers the read apps, the groups left out and each item's key and what
// it creates or changes; wording, source positions and the not-moved
// list are left out, so a harmless difference there does not refuse it.
func PlanHash(p *Plan) string {
	type group struct {
		Name    string `json:"name"`
		Skipped bool   `json:"skipped"`
		Items   []Item `json:"items"`
	}
	approved := struct {
		Snapshots []int64 `json:"snapshots"`
		Groups    []group `json:"groups"`
	}{Snapshots: p.Snapshots}
	for _, g := range p.Groups {
		items := make([]Item, 0, len(g.Items))
		for _, it := range g.Items {
			it.Name, it.From, it.Detail, it.Why = "", "", "", ""
			items = append(items, it)
		}
		approved.Groups = append(approved.Groups, group{Name: g.Name, Skipped: g.Skipped, Items: items})
	}
	b, _ := json.Marshal(approved)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
