package mediaservers

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Plex labels: a movie or show that came into the Library through a request
// gets a label in Plex (the owner's word, and optionally each asker's name),
// so Plex can filter, collect or share by it. Labels already on the item stay;
// a label HolyArrs put on once is never put on again, so one removed in Plex
// stays removed.

// AskedTitle is one asked-for movie or show that is in the Library now.
type AskedTitle struct {
	TitleID int64
	// Askers are the names of the people who asked for it.
	Askers []string
}

// AskedFunc lists the asked-for titles that became available at or after
// since (every one for "").
type AskedFunc func(ctx context.Context, since string) ([]AskedTitle, error)

// maxLabel is the most characters of one label.
const maxLabel = 64

// cleanLabel trims a label and folds runs of spaces; "" when nothing is left.
func cleanLabel(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxLabel {
		s = string([]rune(s)[:maxLabel])
	}
	return s
}

// labelsFor is the labels one asked-for title should carry.
func labelsFor(set LabelSettings, a AskedTitle) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s = cleanLabel(s); s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	add(set.Label)
	if set.Askers {
		for _, n := range a.Askers {
			add(n)
		}
	}
	return out
}

// plexItem is one movie or show on a Plex server.
type plexItem struct {
	section, ratingKey, typ string // typ is Plex's 1 (movie) or 2 (show)
}

// plexItems walks every movie and show on the server matched to a Library
// title by the ids in its Guid tags (and old agents' guid).
func (l *Linker) plexItems(ctx context.Context, m domain.MediaServer, keys map[string]int64, each func(kind string, it plexItem, title int64)) error {
	secs, err := l.plexSections(ctx, m)
	if err != nil {
		return err
	}
	for _, s := range secs {
		kind, typ := "movie", "1"
		switch s.Type {
		case "movie":
		case "show":
			kind, typ = "show", "2"
		default:
			continue
		}
		for start := 0; ; start += watchPage {
			b, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all?type="+typ+"&includeGuids=1"+plexRange(start), nil)
			if err != nil {
				return err
			}
			type entry struct {
				RatingKey string `xml:"ratingKey,attr"`
				GUID      string `xml:"guid,attr"`
				Guids     []struct {
					ID string `xml:"id,attr"`
				} `xml:"Guid"`
			}
			var mc struct {
				Videos      []entry `xml:"Video"`
				Directories []entry `xml:"Directory"`
			}
			if err := xml.Unmarshal(b, &mc); err != nil {
				return errors.New("the server's answer was not a Plex library list")
			}
			list := append(mc.Videos, mc.Directories...)
			for _, d := range list {
				gs := []string{d.GUID}
				for _, g := range d.Guids {
					gs = append(gs, g.ID)
				}
				for _, g := range gs {
					if id := keys[kind+"|"+plexGUID(g)]; id != 0 {
						each(kind, plexItem{section: s.Key, ratingKey: d.RatingKey, typ: typ}, id)
						break
					}
				}
			}
			if len(list) < watchPage {
				break
			}
		}
	}
	return nil
}

// plexLabels reads the labels one Plex item has now.
func (l *Linker) plexLabels(ctx context.Context, m domain.MediaServer, ratingKey string) ([]string, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/library/metadata/"+url.PathEscape(ratingKey), nil)
	if err != nil {
		return nil, err
	}
	type item struct {
		Labels []struct {
			Tag string `xml:"tag,attr"`
		} `xml:"Label"`
	}
	var mc struct {
		Videos      []item `xml:"Video"`
		Directories []item `xml:"Directory"`
	}
	if err := xml.Unmarshal(b, &mc); err != nil {
		return nil, errors.New("the server's answer was not a Plex item")
	}
	var out []string
	for _, it := range append(mc.Videos, mc.Directories...) {
		for _, lb := range it.Labels {
			if lb.Tag != "" {
				out = append(out, lb.Tag)
			}
		}
	}
	return out, nil
}

// putPlexLabels sets an item's labels to have plus add, so labels already on
// it stay.
func (l *Linker) putPlexLabels(ctx context.Context, m domain.MediaServer, it plexItem, have, add []string) error {
	q := url.Values{}
	q.Set("type", it.typ)
	q.Set("id", it.ratingKey)
	q.Set("label.locked", "0")
	for i, lb := range append(append([]string{}, have...), add...) {
		q.Set("label["+strconv.Itoa(i)+"].tag.tag", lb)
	}
	_, err := l.do(ctx, m, http.MethodPut, "/library/sections/"+url.PathEscape(it.section)+"/all?"+q.Encode(), nil)
	return err
}

// LabelPlex puts the labels on asked-for titles on every switched-on Plex
// server: those that became available since labels were turned on, or every
// one when all is set. A title Plex does not have yet is tried again on the
// next pass. It answers how many titles got a label.
func (l *Linker) LabelPlex(ctx context.Context, all bool) (int, error) {
	l.labelling.Lock()
	defer l.labelling.Unlock()
	set := ReadLabelSettings(ctx, l.Store.Read())
	if !set.Enabled || l.Asked == nil {
		return 0, nil
	}
	since := set.Since
	if all {
		since = ""
	}
	asked, err := l.Asked(ctx, since)
	if err != nil || len(asked) == 0 {
		return 0, err
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, m := range ms {
		if m.Kind != "plex" || m.TokenSecretID == nil {
			continue
		}
		got, err := l.labelServer(ctx, m, set, asked)
		n += got
		if err != nil {
			if ctx.Err() != nil {
				return n, ctx.Err()
			}
			log.Printf("media servers: %s: labels: %v", m.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", m.Name, err))
		}
	}
	return n, errors.Join(errs...)
}

// labelServer labels the asked-for titles on one Plex server.
func (l *Linker) labelServer(ctx context.Context, m domain.MediaServer, set LabelSettings, asked []AskedTitle) (int, error) {
	done, err := labelsDone(ctx, l.Store.Read(), m.ID)
	if err != nil {
		return 0, err
	}
	want := map[int64][]string{}
	for _, a := range asked {
		for _, lb := range labelsFor(set, a) {
			if !done[a.TitleID][strings.ToLower(lb)] {
				want[a.TitleID] = append(want[a.TitleID], lb)
			}
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	keys, err := mediafiles.TitlesBySourceID(ctx, l.Store.Read())
	if err != nil {
		return 0, err
	}
	items := map[int64][]plexItem{} // every copy, e.g. Movies and Movies 4K
	if err := l.plexItems(ctx, m, keys, func(_ string, it plexItem, title int64) {
		if _, ok := want[title]; ok {
			items[title] = append(items[title], it)
		}
	}); err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	n := 0
	for _, id := range ids {
		put := false
		for _, it := range items[id] {
			have, err := l.plexLabels(ctx, m, it.ratingKey)
			if err != nil {
				return n, err
			}
			on := map[string]bool{}
			for _, h := range have {
				on[strings.ToLower(h)] = true
			}
			var add []string
			for _, lb := range want[id] {
				if !on[strings.ToLower(lb)] {
					add = append(add, lb)
				}
			}
			if len(add) > 0 {
				if err := l.putPlexLabels(ctx, m, it, have, add); err != nil {
					return n, err
				}
				put = true
			}
		}
		if put {
			n++
		}
		if err := markLabels(ctx, l.Store, m.ID, id, want[id], time.Now().UTC()); err != nil {
			return n, err
		}
	}
	return n, nil
}

// LabelPlexSoon labels every asked-for title now, in the background. Stop
// ends it; Wait waits for it.
func (l *Linker) LabelPlexSoon() {
	if l.Paused {
		return
	}
	l.mu.Lock()
	if l.stopLabel != nil {
		l.stopLabel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	l.stopLabel = cancel
	l.wg.Add(1)
	l.mu.Unlock()
	go func() {
		defer l.wg.Done()
		defer cancel()
		jobs.Guard("media_servers", func() {
			if _, err := l.LabelPlex(ctx, true); err != nil && ctx.Err() == nil {
				log.Printf("media servers: labels: %v", err)
			}
		})
	}()
}

// LabelPass is the scheduled pass: titles that became available since
// labels were turned on.
func (l *Linker) LabelPass(ctx context.Context) error {
	_, err := l.LabelPlex(ctx, false)
	return err
}
