package mediaservers

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const PlaybackPollInterval = 5 * time.Second
const playbackMaxGap = 2 * PlaybackPollInterval

type playbackKey struct {
	Session string
	Account string
	Item    string
}

type playbackSample struct {
	key                   playbackKey
	title, person, device string
	method, state         string
	position, duration    int64
	kind                  string
	guids                 []string
}

type plexPlayback struct {
	Type       string `xml:"type,attr"`
	RatingKey  string `xml:"ratingKey,attr"`
	SessionKey string `xml:"sessionKey,attr"`
	Title      string `xml:"title,attr"`
	Show       string `xml:"grandparentTitle,attr"`
	ShowGUID   string `xml:"grandparentGuid,attr"`
	GUID       string `xml:"guid,attr"`
	Guids      []struct {
		ID string `xml:"id,attr"`
	} `xml:"Guid"`
	Position int64 `xml:"viewOffset,attr"`
	Duration int64 `xml:"duration,attr"`
	User     struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"title,attr"`
	} `xml:"User"`
	Player struct {
		Name  string `xml:"title,attr"`
		State string `xml:"state,attr"`
	} `xml:"Player"`
	Session struct {
		ID string `xml:"id,attr"`
	} `xml:"Session"`
	Transcode struct {
		Video string `xml:"videoDecision,attr"`
		Audio string `xml:"audioDecision,attr"`
	} `xml:"TranscodeSession"`
	Media []struct {
		Selected bool `xml:"selected,attr"`
		Parts    []struct {
			Selected bool   `xml:"selected,attr"`
			Decision string `xml:"decision,attr"`
		} `xml:"Part"`
	} `xml:"Media"`
}

func (p plexPlayback) method() string {
	if p.Transcode.Video == "transcode" || p.Transcode.Audio == "transcode" {
		return "transcode"
	}
	for _, media := range p.Media {
		if len(p.Media) > 1 && !media.Selected {
			continue
		}
		for _, part := range media.Parts {
			if len(media.Parts) > 1 && !part.Selected {
				continue
			}
			switch strings.ToLower(part.Decision) {
			case "directplay":
				return "direct_play"
			case "copy", "directstream":
				return "direct_stream"
			case "transcode":
				return "transcode"
			}
		}
	}
	if p.Transcode.Video == "copy" || p.Transcode.Audio == "copy" {
		return "direct_stream"
	}
	return "unknown"
}

func parsePlayback(body []byte) (map[playbackKey]playbackSample, error) {
	var container struct {
		XMLName xml.Name       `xml:"MediaContainer"`
		Size    *int           `xml:"size,attr"`
		Videos  []plexPlayback `xml:"Video"`
	}
	if err := xml.Unmarshal(body, &container); err != nil || container.Size == nil || *container.Size < len(container.Videos) {
		return nil, errors.New("the server's answer was not a Plex sessions list")
	}
	plays := make(map[playbackKey]playbackSample)
	for _, p := range container.Videos {
		if p.Type != "movie" && p.Type != "episode" {
			continue
		}
		key := playbackKey{Session: p.Session.ID, Account: p.User.ID, Item: p.RatingKey}
		if key.Session == "" {
			key.Session = p.SessionKey
		}
		if key.Session == "" || key.Account == "" || key.Item == "" || len(key.Session) > 160 || len(key.Account) > 160 || len(key.Item) > 160 {
			return nil, errors.New("a Plex session did not identify the play")
		}
		sample := playbackSample{key: key, title: p.Title, person: p.User.Name, device: p.Player.Name, method: p.method(),
			state: playbackPlayerState(p.Player.State), position: max(0, p.Position), duration: max(0, p.Duration), kind: "movie", guids: []string{p.GUID}}
		for _, g := range p.Guids {
			sample.guids = append(sample.guids, g.ID)
		}
		if p.Type == "episode" {
			sample.kind, sample.guids = "show", []string{p.ShowGUID}
			if p.Show != "" {
				sample.title = p.Show + " · " + p.Title
			}
		}
		plays[key] = sample
	}
	return plays, nil
}

func playbackPlayerState(state string) string {
	switch state {
	case "playing", "paused", "buffering":
		return state
	default:
		return "unknown"
	}
}

// PollPlayback observes only the Plex connections whose owners enabled monitoring.
func (l *Linker) PollPlayback(ctx context.Context) error {
	servers, err := playbackServers(ctx, l.Store.Read())
	if err != nil {
		return err
	}
	var errs []error
	for _, server := range servers {
		if err := l.pollPlaybackSource(ctx, server); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (l *Linker) pollPlaybackSource(ctx context.Context, server playbackSource) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	request, cancel := context.WithTimeout(ctx, 4*time.Second)
	body, err := l.do(request, server.MediaServer, http.MethodGet, "/status/sessions?includeGuids=1", nil) // External API path
	cancel()
	if err != nil {
		return storePlaybackUnavailable(ctx, l.Store, server.ID, server.Generation)
	}
	samples, err := parsePlayback(body)
	if err != nil {
		return storePlaybackUnavailable(ctx, l.Store, server.ID, server.Generation)
	}
	previous, _ := l.playbackSeen.Load(server.ID)
	applied, err := savePlaybackSnapshot(ctx, l.Store, server.ID, server.Generation, samples, time.Now(), previous == server.Generation)
	if applied {
		l.playbackSeen.Store(server.ID, server.Generation)
	}
	return err
}

func playbackTitle(sample playbackSample, keys map[string]int64) *int64 {
	for _, raw := range sample.guids {
		if id := keys[sample.kind+"|"+plexGUID(raw)]; id != 0 {
			return &id
		}
	}
	return nil
}

func playbackText(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	return string(runes[:min(len(runes), limit)])
}

func playbackOffset(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, errors.New("invalid playback history offset")
	}
	return n, nil
}
