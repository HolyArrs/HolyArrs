package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// maxPast is how many past downloads, imports and failed downloads are
// brought from one old app: the newest ones. An app that ran for years can
// hold far more, mostly old grabs nobody looks back at.
const maxPast = 5000

// past is the set the history is kept in, named in a note.
var pastList = arrSet{name: "history"}

// PastEvent is one line of the old app's history that comes over: a
// download it sent, one it imported, or one that failed.
type PastEvent struct {
	OldID   int64  `json:"old_id"`
	Kind    string `json:"kind"` // the Event kind here: download_sent, import_done, download_failed
	At      string `json:"at"`   // RFC 3339
	Release string `json:"release,omitempty"`
	Quality string `json:"quality,omitempty"`
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Source  string `json:"source,omitempty"`     // the indexer's name
	Getter  string `json:"downloader,omitempty"` // the download client's name
	Reason  string `json:"reason,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// pastKinds are the old apps' history kinds that come over, by their API
// name and by their number in a backup (Sonarr and Radarr share 1, 3, 4).
var pastKinds = map[string]string{"grabbed": "download_sent", "downloadfolderimported": "import_done", "downloadfailed": "download_failed",
	"1": "download_sent", "3": "import_done", "4": "download_failed"}

func (r arrReader) pastLink() string {
	return map[Kind]string{Radarr: "movieid", Sonarr: "seriesid"}[r.kind]
}

// pastAPI reads the newest history, page by page, keeping only what comes
// over. History is a help: a failure is a note, never a failed read.
func (r arrReader) pastAPI(ctx context.Context, c *apiClient, out *arrRows) {
	link := r.pastLink()
	if link == "" {
		return
	}
	var rows []map[string]any
	for page := 1; page <= 50 && len(rows) < maxPast && ctx.Err() == nil; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {"500"}, "sortKey": {"date"}, "sortDirection": {"descending"}}
		if r.kind == Sonarr {
			q.Set("includeEpisode", "true")
		}
		var env map[string]any
		if err := c.getJSON(ctx, r.apiBase()+"/history", q, &env); err != nil {
			if len(rows) == 0 {
				out.notes = append(out.notes, "the app has no "+pastList.name+" list; skipped")
				return
			}
			// A later page failing keeps the newer pages already read.
			out.notes = append(out.notes, "the app's "+pastList.name+" list stopped early; brought what was read")
			break
		}
		recs := list(lower(env)["records"])
		for _, e := range recs {
			m := mapOf(e)
			if m == nil {
				continue
			}
			kind := pastKinds[strings.ToLower(str(m["eventtype"]))]
			if kind == "" {
				continue
			}
			d := mapOf(m["data"])
			ep := mapOf(m["episode"])
			rows = append(rows, map[string]any{"titleid": m[link], "past": PastEvent{OldID: num(m["id"]), Kind: kind, At: str(m["date"]),
				Release: str(m["sourcetitle"]), Quality: qualityName(m["quality"], nil), Season: smallInt(num(ep["seasonnumber"])), Episode: smallInt(num(ep["episodenumber"])),
				Source: str(d["indexer"]), Getter: firstStr(d, "downloadclientname", "downloadclient"), Reason: str(d["message"]), Size: num(d["size"])}})
		}
		if len(recs) < 500 {
			break
		}
	}
	out.sets["past"] = rows
}

// pastBackup reads the same from a backup's History table, newest first.
func (r arrReader) pastBackup(ctx context.Context, t table, out *arrRows) {
	link := r.pastLink()
	cols, err := t.columns(ctx, "History")
	if link == "" || err != nil || cols["eventtype"] == "" || cols[link] == "" {
		return
	}
	defs := map[int64]string{}
	for _, m := range out.sets["qualitydefs"] {
		defs[num(m["quality"])] = str(m["title"])
	}
	eps := map[int64][2]int{}
	if r.kind == Sonarr && cols["episodeid"] != "" {
		if er, err := t.db.QueryContext(ctx, "SELECT Id, SeasonNumber, EpisodeNumber FROM Episodes"); err == nil {
			for er.Next() {
				var id int64
				var s, n int
				if er.Scan(&id, &s, &n) == nil {
					eps[id] = [2]int{s, n}
				}
			}
			er.Close()
		}
	}
	epCol := "0"
	if r.kind == Sonarr && cols["episodeid"] != "" {
		epCol = cols["episodeid"]
	}
	q := "SELECT Id, " + cols[link] + ", " + epCol + ", COALESCE(SourceTitle,''), COALESCE(Date,''), COALESCE(Quality,''), COALESCE(Data,''), EventType FROM History WHERE EventType IN (1,3,4) ORDER BY Date DESC LIMIT " + strconv.Itoa(maxPast)
	hr, err := t.db.QueryContext(ctx, q)
	if err != nil {
		out.notes = append(out.notes, "the app has no "+pastList.name+" list; skipped")
		return
	}
	defer hr.Close()
	var rows []map[string]any
	for hr.Next() {
		var id, title, ep, kind int64
		var release, at, quality, data string
		if hr.Scan(&id, &title, &ep, &release, &at, &quality, &data, &kind) != nil {
			continue
		}
		var d map[string]any
		_ = json.Unmarshal([]byte(data), &d)
		d = lower(d)
		p := PastEvent{OldID: id, Kind: pastKinds[strconv.FormatInt(kind, 10)], At: at, Release: release, Quality: qualityName(quality, defs),
			Source: str(d["indexer"]), Getter: firstStr(d, "downloadclientname", "downloadclient"), Reason: str(d["message"]), Size: num(d["size"])}
		if e, ok := eps[ep]; ok {
			p.Season, p.Episode = e[0], e[1]
		}
		rows = append(rows, map[string]any{"titleid": title, "past": p})
	}
	out.sets["past"] = rows
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

// pastByTitle groups the history by the old app's title id.
func pastByTitle(rows []map[string]any) map[int64][]PastEvent {
	out := map[int64][]PastEvent{}
	for _, m := range rows {
		if p, ok := m["past"].(PastEvent); ok && p.Kind != "" {
			out[num(m["titleid"])] = append(out[num(m["titleid"])], p)
		}
	}
	return out
}

// pastGroupKey marks an Event brought from an old app, so moving again
// never adds it twice and undoing the import finds it.
func pastGroupKey(app string, p PastEvent) string {
	return "moved:" + app + ":" + strconv.FormatInt(p.OldID, 10)
}

// bringPast adds a title's past downloads to its History. They are written
// straight to the events database, so no Alert, media server update or
// webhook fires for things that happened long ago. It returns how many
// were added.
func (r *run) bringPast(titleID int64, t *TitlePlan) int {
	if len(t.History) == 0 || t.App == "" {
		return 0
	}
	have := map[string]bool{}
	if rows, err := r.m.Store.ReadEvents().QueryContext(r.ctx, "SELECT group_key FROM events WHERE title_id = ? AND group_key LIKE 'moved:%'", titleID); err == nil {
		for rows.Next() {
			var k string
			if rows.Scan(&k) == nil {
				have[k] = true
			}
		}
		rows.Close()
	}
	items := map[[2]int]int64{}
	if t.Kind == "show" {
		if rows, err := r.m.Store.Read().QueryContext(r.ctx, "SELECT id, COALESCE(json_extract(numbers, '$.season'), -1), COALESCE(json_extract(numbers, '$.episode'), -1) FROM items WHERE title_id = ? AND kind = 'episode'", titleID); err == nil {
			for rows.Next() {
				var id int64
				var s, n int
				if rows.Scan(&id, &s, &n) == nil {
					items[[2]int{s, n}] = id
				}
			}
			rows.Close()
		}
	}
	app := map[string]string{"sonarr": "Sonarr", "radarr": "Radarr"}[t.App]
	var add []database.Event
	for _, p := range t.History {
		key := pastGroupKey(t.App, p)
		at, err := time.Parse(time.RFC3339Nano, p.At)
		if err != nil {
			at, err = time.Parse("2006-01-02 15:04:05.9999999Z", p.At)
		}
		if err != nil {
			at, err = time.Parse("2006-01-02 15:04:05Z07:00", p.At)
		}
		if err != nil {
			at, err = time.Parse("2006-01-02 15:04:05", p.At)
		}
		if err != nil || have[key] {
			continue
		}
		have[key] = true
		id := titleID
		e := database.Event{At: at.UTC(), Actor: database.System, TitleID: &id, GroupKey: key,
			Data: map[string]any{"moved_from": app, "move_id": r.move, "release": p.Release}}
		if p.Quality != "" {
			e.Data["quality"] = p.Quality
		}
		if p.Season > 0 || p.Episode > 0 {
			e.Data["season"], e.Data["episode"] = p.Season, p.Episode
			if it := items[[2]int{p.Season, p.Episode}]; it != 0 {
				e.Data["item_ids"] = []int64{it}
			}
		}
		switch p.Kind {
		case "download_sent":
			e.Kind, e.Summary = p.Kind, "Sent "+p.Release
			if p.Getter != "" {
				e.Summary += " to " + p.Getter
			}
			if p.Source != "" {
				e.Data["source"] = p.Source
			}
		case "import_done":
			e.Kind, e.Summary = p.Kind, "Imported "+p.Release
			if p.Size > 0 {
				e.Data["size"] = p.Size
			}
		case "download_failed":
			e.Kind, e.Summary = p.Kind, "Download failed: "+p.Release
			if p.Reason != "" {
				e.Data["reason"] = p.Reason
			}
		default:
			continue
		}
		add = append(add, e)
	}
	if len(add) == 0 {
		return 0
	}
	err := r.m.Store.WriteEvents(r.ctx, func(tx *sql.Tx) error {
		for i := range add {
			if _, err := database.Insert(tx, "events", &add[i], r.by); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0
	}
	return len(add)
}

// undoPast takes the history an import brought out again.
func (m *Mover) undoPast(ctx context.Context, moveID int64) error {
	return m.Store.WriteEvents(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM events WHERE group_key LIKE 'moved:%' AND json_extract(data, '$.move_id') = ?", moveID)
		return err
	})
}

// blockedEpisodesBackup reads the numbers of the blocklist's episode ids
// from a Sonarr backup (see blocklist_episodes.go); it sits here with the
// other Episodes table read.
func (r arrReader) blockedEpisodesBackup(ctx context.Context, t table, out *arrRows) {
	want := r.blockedIDs(out)
	if len(want) == 0 || !t.exists(ctx, "Episodes") {
		return
	}
	nums := map[int64]sources.EpisodeNumber{}
	rows, err := t.db.QueryContext(ctx, "SELECT Id, SeasonNumber, EpisodeNumber FROM Episodes")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n sources.EpisodeNumber
		if rows.Scan(&id, &n.Season, &n.Episode) == nil && slices.Contains(want, id) {
			nums[id] = n
		}
	}
	setBlockedEpisodes(out, nums)
}
