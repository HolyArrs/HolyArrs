package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Plex friends: with "Include Plex friends" on, the watchlists of people
// imported from the owner's Plex account are read with the owner's Plex link
// and asked for as them, through the same path as anyone's watchlist. Only
// people already in HolyArrs are read, and only while Plex still lists them
// among the owner's friends who share the chosen server. A friend who turned
// their watchlist off is left alone, and one read with their own Plex
// sign-in is not read twice. Kids keep their Kid limits.

// DefaultPlexTV is where Plex lists the owner's friends.
const DefaultPlexTV = "https://plex.tv"

// DefaultPlexCommunity is Plex's community service, where friends'
// watchlists are read.
const DefaultPlexCommunity = "https://community.plex.tv"

// friendWords are the notes a friend's pass leaves on their watchlist card.
var friendWords = map[string]string{
	"gone":    "This Plex account no longer shares the owner's server, so its watchlist is not read.",
	"empty":   "Plex showed an empty watchlist. If it has titles, its privacy setting may hide it from friends.",
	"hidden":  "Plex did not share this watchlist with the owner's Plex account. Its privacy setting may hide it from friends.",
	"refused": "Plex refused the owner's Plex link. The owner can link Plex again in Settings.",
}

var errFriendHidden = errors.New(friendWords["hidden"])

// plexUUID is the shape of a Plex account's uuid; nothing else goes into a
// watchlist query.
var plexUUID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

func (w *Watchlists) plexTV() string {
	if w.PlexTV != "" {
		return strings.TrimRight(w.PlexTV, "/")
	}
	return DefaultPlexTV
}

func (w *Watchlists) community() string {
	if w.Community != "" {
		return strings.TrimRight(w.Community, "/")
	}
	return DefaultPlexCommunity
}

// friendsLink is the owner's Plex token and chosen server while Plex friends
// are read; the token is "" when they are not.
func (w *Watchlists) friendsLink(ctx context.Context) (tok, server string, err error) {
	set, err := ReadSettings(ctx, w.Asks.Store.Read())
	if err != nil || !set.PlexWatchlists || !set.PlexFriends || w.Owner == nil {
		return "", "", err
	}
	tok, server = w.Owner(ctx)
	return tok, server, nil
}

// friendsDue says whether a pass has a Plex friend to read. It calls nobody.
func (w *Watchlists) friendsDue(ctx context.Context) (bool, error) {
	tok, _, err := w.friendsLink(ctx)
	if err != nil || tok == "" {
		return false, err
	}
	var n int
	err = w.Asks.Store.Read().QueryRowContext(ctx, friendsDueSQL).Scan(&n)
	return n > 0, err
}

// friendRow is a person with a Plex link who may be read as a friend.
type friendRow struct {
	userID int64
	plexID string
}

// friendsResult is what one pass over Plex friends did.
type friendsResult struct {
	PassResult
	People int
}

// friendsPass reads the watchlists of imported Plex friends with the owner's
// Plex link. covered are people already read with their own sign-in.
func (w *Watchlists) friendsPass(ctx context.Context, covered map[int64]bool) (friendsResult, error) {
	var out friendsResult
	tok, server, err := w.friendsLink(ctx)
	if err != nil || tok == "" {
		return out, err
	}
	rows, err := w.Asks.Store.Read().QueryContext(ctx, friendsSQL)
	if err != nil {
		return out, err
	}
	var todo []friendRow
	for rows.Next() {
		var f friendRow
		if err := rows.Scan(&f.userID, &f.plexID); err != nil {
			rows.Close()
			return out, err
		}
		if !covered[f.userID] {
			todo = append(todo, f)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	todo = slices.DeleteFunc(todo, func(f friendRow) bool { return !w.mayAskAs(ctx, f.userID) })
	if len(todo) == 0 {
		return out, nil
	}
	listed, err := w.fetchFriends(ctx, tok, server)
	if err != nil {
		return out, err
	}
	for _, f := range todo {
		uuid, ok := listed[f.plexID]
		if !ok {
			if err := w.noteGone(ctx, f.userID); err != nil {
				return out, err
			}
			continue
		}
		out.People++
		res, err := w.friendPass(ctx, f.userID, uuid, tok)
		if err != nil {
			log.Printf("plex watchlist: friend %d: %v", f.userID, err)
			out.Failed++
		}
		out.Read += res.Read
		out.Asked += res.Asked
	}
	return out, nil
}

// mayAskAs says whether userID is active and may ask for titles, so their
// watchlist is worth reading.
func (w *Watchlists) mayAskAs(ctx context.Context, userID int64) bool {
	var name, role, perms, state, maxRating string
	var movies, shows, chosen bool
	err := w.Asks.Store.Read().QueryRowContext(ctx, passSQL, userID).Scan(&name, &role, &perms, &state, &maxRating, &movies, &shows, &chosen)
	return err == nil && state == "active" && personOf(userID, name, role, perms, maxRating) != nil
}

// noteGone tells a person the friends path read before that Plex no longer
// lists them. People it never read, or who keep their own Plex sign-in,
// get no note, and an unchanged note is not written again.
func (w *Watchlists) noteGone(ctx context.Context, userID int64) error {
	if w.HasToken(ctx, userID) {
		return nil
	}
	st, err := ReadWatchlist(ctx, w.Asks.Store.Read(), userID)
	if err != nil || st.Error == friendWords["gone"] {
		return err
	}
	readBefore := len(st.Items) > 0 || st.CheckedAt != nil && (st.Error == "" || slices.Contains([]string{friendWords["empty"], friendWords["hidden"], friendWords["refused"]}, st.Error))
	if !readBefore {
		return nil
	}
	return w.checked(ctx, userID, friendWords["gone"])
}

// friendPass reads one imported friend's watchlist with the owner's token
// and acts on its new entries as them. Unless they chose for themselves,
// movies and shows are both read.
func (w *Watchlists) friendPass(ctx context.Context, userID int64, uuid, tok string) (PassResult, error) {
	var res PassResult
	var name, role, perms, state, maxRating string
	var movies, shows, chosen bool
	err := w.Asks.Store.Read().QueryRowContext(ctx, passSQL, userID).Scan(&name, &role, &perms, &state, &maxRating, &movies, &shows, &chosen)
	if errors.Is(err, sql.ErrNoRows) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if !chosen {
		movies, shows = true, true
	}
	if !movies && !shows || state != "active" {
		return res, nil
	}
	p := personOf(userID, name, role, perms, maxRating)
	if p == nil {
		return res, nil
	}
	entries, err := w.fetchFriendList(ctx, tok, uuid)
	if err != nil {
		note := err.Error()
		switch {
		case errors.Is(err, errPlexForbidden):
			note = friendWords["hidden"]
		case errors.Is(err, ErrPlexRefused):
			note = friendWords["refused"]
		}
		if err := w.checked(ctx, userID, note); err != nil {
			return res, err
		}
		if errors.Is(err, errFriendHidden) || errors.Is(err, errPlexForbidden) {
			return res, nil // their privacy choice, not a failure
		}
		return res, err
	}
	if len(entries) == 0 {
		return res, w.checked(ctx, userID, friendWords["empty"])
	}
	return w.actOn(ctx, userID, *p, movies, shows, tok, entries)
}

// fetchFriends maps each Plex friend's account id to their uuid, keeping
// only those who share server ("" for any). With a server chosen, a friend
// Plex lists without servers is kept only when they are in the owner's Plex
// Home: a friend whose share was removed can be listed that way too.
func (w *Watchlists) fetchFriends(ctx context.Context, tok, server string) (map[string]string, error) {
	var got []struct {
		ID     flexText `json:"id"`
		UUID   string   `json:"uuid"`
		Home   bool     `json:"home"`
		Shared []struct {
			MachineIdentifier string `json:"machineIdentifier"`
		} `json:"sharedServers"`
	}
	base := w.plexTV()
	if err := w.call(ctx, http.MethodGet, base+"/api/v2/friends", tok, nil, &got, base, tok); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range got {
		id, uuid := strings.TrimSpace(string(f.ID)), strings.TrimSpace(f.UUID)
		if id == "" || !plexUUID.MatchString(uuid) {
			continue
		}
		if server != "" && len(f.Shared) == 0 && !f.Home {
			continue
		}
		if server != "" && len(f.Shared) > 0 && !slices.ContainsFunc(f.Shared, func(s struct {
			MachineIdentifier string `json:"machineIdentifier"`
		}) bool {
			return s.MachineIdentifier == server
		}) {
			continue
		}
		out[id] = uuid
	}
	return out, nil
}

// fetchFriendList reads the newest entries of a friend's watchlist. The
// answer has no ids; each new entry is looked up once, as for anyone.
func (w *Watchlists) fetchFriendList(ctx context.Context, tok, uuid string) ([]watchEntry, error) {
	if !plexUUID.MatchString(uuid) {
		return nil, errFriendHidden
	}
	q := `query { user(id: "` + uuid + `") { watchlist(first: ` + strconv.Itoa(watchlistRead) + `) { nodes { id title type } } } }`
	body, err := json.Marshal(map[string]string{"query": q})
	if err != nil {
		return nil, err
	}
	var got struct {
		Data struct {
			User *struct {
				Watchlist *struct {
					Nodes []struct {
						ID    flexText `json:"id"`
						Title string   `json:"title"`
						Type  string   `json:"type"`
					} `json:"nodes"`
				} `json:"watchlist"`
			} `json:"user"`
		} `json:"data"`
	}
	base := w.community()
	if err := w.call(ctx, http.MethodPost, base+"/api", tok, body, &got, base, tok, uuid); err != nil {
		return nil, err
	}
	if got.Data.User == nil || got.Data.User.Watchlist == nil {
		return nil, errFriendHidden
	}
	out := []watchEntry{}
	for _, n := range got.Data.User.Watchlist.Nodes {
		kind := map[string]string{"movie": "movie", "show": "show"}[strings.ToLower(n.Type)]
		key := strings.TrimSpace(string(n.ID))
		if kind == "" || key == "" {
			continue
		}
		out = append(out, watchEntry{key: key, kind: kind, name: strings.TrimSpace(n.Title), ids: map[string]string{}})
	}
	return out, nil
}

// PlexFriend is one person with a Plex link as the owner's Request rules
// list them: whether they are a Kid, their last read and what it asked for.
type PlexFriend struct {
	UserID    int64      `json:"user_id"`
	Name      string     `json:"name"`
	PlexName  string     `json:"plex_name"`
	Kid       bool       `json:"kid"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Error     string     `json:"error,omitempty"`
	// Asked counts the entries asked for or joined from their watchlist.
	Asked int `json:"asked"`
}

// ReadPlexFriends lists the active people with a Plex link, the owner left
// out, by name.
func ReadPlexFriends(ctx context.Context, db *sql.DB) ([]PlexFriend, error) {
	rows, err := db.QueryContext(ctx, readPlexFriendsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlexFriend{}
	for rows.Next() {
		var f PlexFriend
		var at sql.NullString
		if err := rows.Scan(&f.UserID, &f.Name, &f.PlexName, &f.Kid, &at, &f.Error, &f.Asked); err != nil {
			return nil, err
		}
		if at.Valid {
			t, _ := time.Parse(database.TimeLayout, at.String)
			f.CheckedAt = &t
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
