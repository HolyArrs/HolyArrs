package fakesite

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OpenSubs fakes the shape of the OpenSubtitles.com API: an Api-Key on every
// call, a sign-in that returns a token and a daily download count. Every key,
// name and password it knows is made up by the test that sets it, and it
// never contacts the outside.
type OpenSubs struct {
	mu       sync.Mutex
	Key      string
	User     string
	Pass     string
	Subs     []Sub // Kind is movie or episode; ID is the numeric file id
	Quota    int   // downloads left today
	Logins   int
	Searches int
	// Expire makes the next download refuse the current token once, as if it
	// had expired.
	Expire bool
	// Hostile, when set, is the raw body of every search answer.
	Hostile string
	// LinkHost, when set, is the host download links point at instead of the
	// fake itself.
	LinkHost string
	tokens   map[string]bool
	n        int
}

func (o *OpenSubs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/files/") {
		o.file(w, strings.TrimPrefix(r.URL.Path, "/files/"))
		return
	}
	if o.Key != "" && r.Header.Get("Api-Key") != o.Key {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "You cannot consume this service"})
		return
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/login"):
		o.Logins++
		var in struct{ Username, Password string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || in.Username != o.User || in.Password != o.Pass {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Error, invalid username/password combination.", "status": 401})
			return
		}
		o.n++
		tok := fmt.Sprintf("fake-token-%d", o.n)
		if o.tokens == nil {
			o.tokens = map[string]bool{}
		}
		o.tokens[tok] = true
		writeJSON(w, http.StatusOK, map[string]any{"token": tok, "status": 200, "user": map[string]any{"allowed_downloads": o.Quota, "vip": false}})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/subtitles"):
		o.Searches++
		if o.Hostile != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(o.Hostile))
			return
		}
		o.search(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/download"):
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if o.Expire && o.tokens[tok] {
			o.Expire = false
			delete(o.tokens, tok)
		}
		if !o.tokens[tok] {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "token expired"})
			return
		}
		reset := time.Now().UTC().Add(3 * time.Hour).Format(time.RFC3339)
		if o.Quota <= 0 {
			writeJSON(w, http.StatusNotAcceptable, map[string]any{"message": "You have downloaded your allowed 20 subtitles for 24h", "remaining": 0, "reset_time_utc": reset})
			return
		}
		var in struct {
			FileID int64 `json:"file_id"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
		o.Quota--
		host := o.LinkHost
		if host == "" {
			host = "http://" + r.Host
		}
		writeJSON(w, http.StatusOK, map[string]any{"link": host + "/files/" + strconv.FormatInt(in.FileID, 10), "remaining": o.Quota, "reset_time_utc": reset})
	default:
		http.NotFound(w, r)
	}
}

func (o *OpenSubs) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	langs := strings.Split(q.Get("languages"), ",")
	data := []any{}
	for _, s := range o.Subs {
		if s.Kind != q.Get("type") || !contains(langs, strings.SplitN(s.Language, "-", 2)[0]) {
			continue
		}
		feature := map[string]any{"title": s.Title, "year": s.Year}
		if s.Kind == "episode" {
			if strconv.Itoa(s.Season) != q.Get("season_number") || strconv.Itoa(s.Episode) != q.Get("episode_number") {
				continue
			}
			feature = map[string]any{"title": "Episode", "parent_title": s.Title, "year": s.Year, "season_number": s.Season, "episode_number": s.Episode}
			if s.ByID {
				feature["parent_imdb_id"], _ = strconv.Atoi(q.Get("parent_imdb_id"))
			}
		} else if s.ByID {
			feature["imdb_id"], _ = strconv.Atoi(q.Get("imdb_id"))
		}
		id, _ := strconv.ParseInt(s.ID, 10, 64)
		data = append(data, map[string]any{"id": "s" + s.ID, "type": "subtitle", "attributes": map[string]any{
			"language": s.Language, "hearing_impaired": s.HearingImpaired, "foreign_parts_only": s.Forced, "fps": s.FPS,
			"download_count": s.Downloads, "release": s.Release, "from_trusted": s.Trusted, "moviehash_match": s.Corrected && q.Get("moviehash") != "",
			"uploader": map[string]any{"name": s.Uploader}, "feature_details": feature,
			"files": []any{map[string]any{"file_id": id, "file_name": s.Release + ".srt"}},
		}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total_count": len(data), "data": data})
}

func (o *OpenSubs) file(w http.ResponseWriter, id string) {
	for _, s := range o.Subs {
		if s.ID == id {
			w.Header().Set("Content-Type", "application/x-subrip")
			_, _ = w.Write([]byte(s.Text))
			return
		}
	}
	http.NotFound(w, nil)
}

// Subdl fakes the shape of the subdl API: the key in the query, search answers
// with zip links and a daily download count. Every value is made up.
type Subdl struct {
	mu        sync.Mutex
	Key       string
	Subs      []Sub // ID is the zip file name
	Quota     int
	Searches  int
	Downloads int
	// Hostile, when set, is the raw body of every search answer.
	Hostile string
}

func (s *Subdl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, "/subtitles"):
		s.Searches++
		if r.URL.Query().Get("api_key") != s.Key {
			writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Invalid API key"})
			return
		}
		if s.Hostile != "" {
			_, _ = w.Write([]byte(s.Hostile))
			return
		}
		s.search(w, r)
	case strings.HasPrefix(r.URL.Path, "/subtitle/"):
		if s.Quota <= 0 {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"status": false, "error": "daily download limit reached"})
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/subtitle/"), ".zip")
		for _, x := range s.Subs {
			if x.ID == id {
				s.Quota--
				s.Downloads++
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				f, _ := zw.Create(x.Release + ".srt")
				_, _ = f.Write([]byte(x.Text))
				_ = zw.Close()
				w.Header().Set("Content-Type", "application/zip")
				_, _ = w.Write(buf.Bytes())
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Subdl) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	langs := strings.Split(strings.ToLower(q.Get("languages")), ",")
	kind := "movie"
	if q.Get("type") == "tv" {
		kind = "episode"
	}
	subs := []any{}
	var title string
	var year int
	for _, x := range s.Subs {
		if x.Kind != kind || !contains(langs, x.Language) {
			continue
		}
		if kind == "episode" && strconv.Itoa(x.Season) != q.Get("season_number") {
			continue
		}
		if q.Get("film_name") != "" && fold(q.Get("film_name")) != fold(x.Title) {
			continue
		}
		title, year = x.Title, x.Year
		subs = append(subs, map[string]any{"release_name": x.Release, "name": "SUBDL::" + x.Release + ".zip", "lang": "english",
			"language": strings.ToUpper(x.Language), "author": x.Uploader, "url": "/subtitle/" + x.ID + ".zip", "season": x.Season,
			"episode": x.Episode, "hi": x.HearingImpaired, "full_season": false})
	}
	if len(subs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "can't find movie or tv"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "results": []any{map[string]any{"name": title, "year": year}}, "subtitles": subs})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
