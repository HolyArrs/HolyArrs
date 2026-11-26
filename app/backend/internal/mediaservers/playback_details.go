package mediaservers

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type plexPlaybackMedia struct {
	Selected   bool       `xml:"selected,attr"`
	Resolution string     `xml:"videoResolution,attr"`
	VideoCodec string     `xml:"videoCodec,attr"`
	AudioCodec string     `xml:"audioCodec,attr"`
	Parts      []plexPlaybackPart `xml:"Part"`
}

type plexPlaybackPart struct {
	Selected bool   `xml:"selected,attr"`
	Decision string `xml:"decision,attr"`
	File     string `xml:"file,attr"`
	Streams  []struct {
		Type     int    `xml:"streamType,attr"`
		Selected bool   `xml:"selected,attr"`
		Decision string `xml:"decision,attr"`
	} `xml:"Stream"`
}

type streamDetails struct {
	SourceResolution string `json:"source_resolution,omitempty"`
	SourceVideo      string `json:"source_video,omitempty"`
	SourceAudio      string `json:"source_audio,omitempty"`
	OutputHeight     int    `json:"output_height,omitempty"`
	OutputVideo      string `json:"output_video,omitempty"`
	OutputAudio      string `json:"output_audio,omitempty"`
	VideoDecision    string `json:"video_decision,omitempty"`
	AudioDecision    string `json:"audio_decision,omitempty"`
	SubtitleDecision string `json:"subtitle_decision,omitempty"`
	Connection       string `json:"connection,omitempty"`
	BandwidthKbps    int    `json:"bandwidth_kbps,omitempty"`
	Hardware         string `json:"hardware,omitempty"`
	BufferMS         int64  `json:"buffer_ms"`
	BufferEvents     int    `json:"buffer_events"`
}

func chosenPlex[T any](items []T, selected func(T) bool) T {
	if len(items) == 1 {
		return items[0]
	}
	var result T
	found := false
	for _, item := range items {
		if !selected(item) {
			continue
		}
		if found {
			var zero T
			return zero
		}
		found, result = true, item
	}
	return result
}

var plexThumbPath = regexp.MustCompile(`^/library/metadata/[0-9]{1,18}/thumb(?:/[0-9]{1,18})?$`)

func (p plexPlayback) details(media plexPlaybackMedia, part plexPlaybackPart) streamDetails {
	x := p.Transcode
	field := func(s string) string { return playbackText(s, 40) }
	d := streamDetails{
		SourceResolution: field(media.Resolution), SourceVideo: field(media.VideoCodec), SourceAudio: field(media.AudioCodec),
		OutputHeight: min(max(x.Height, 0), 16384), OutputVideo: field(x.VideoCodec), OutputAudio: field(x.AudioCodec),
		VideoDecision: field(x.Video), AudioDecision: field(x.Audio), SubtitleDecision: field(x.Subtitle),
		BandwidthKbps: max(p.Session.Bandwidth, 0),
	}
	for _, stream := range part.Streams {
		if !stream.Selected {
			continue
		}
		switch stream.Type {
		case 1:
			if d.VideoDecision == "" {
				d.VideoDecision = field(stream.Decision)
			}
		case 2:
			if d.AudioDecision == "" {
				d.AudioDecision = field(stream.Decision)
			}
		case 3:
			if d.SubtitleDecision == "" {
				d.SubtitleDecision = field(stream.Decision)
			}
		}
	}
	switch {
	case p.Player.Relayed == "1" || p.Player.Relayed == "true":
		d.Connection = "relay"
	case p.Session.Location == "lan" || p.Session.Location == "wan":
		d.Connection = p.Session.Location
	}
	switch strings.ToLower(x.Hardware) {
	case "", "0", "false":
	default:
		d.Hardware = field(x.Hardware)
	}
	return d
}

type playbackFile struct {
	id, title, version int64
}

// Match only a unique present media file using the same path mapping that
// HolyArrs uses to tell Plex about an import. Never open the path sent by Plex.
func matchPlaybackFile(ctx context.Context, q *sql.Tx, server domain.MediaServer, plexFile string, title *int64) (playbackFile, error) {
	if plexFile == "" {
		return playbackFile{}, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT f.id, v.title_id, v.id, lf.path, f.relative_path
		FROM versions v JOIN files f ON f.version_id = v.id
		JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE f.state = 'present' AND f.role = 'main' AND (? IS NULL OR v.title_id = ?)`, title, title)
	if err != nil {
		return playbackFile{}, err
	}
	defer rows.Close()
	fold := winPath(plexFile)
	normalize := func(p string) string {
		if fold {
			return strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
		}
		return p
	}
	want := normalize(plexFile)
	var match playbackFile
	for rows.Next() {
		var candidate playbackFile
		var root, rel string
		if err := rows.Scan(&candidate.id, &candidate.title, &candidate.version, &root, &rel); err != nil {
			return playbackFile{}, err
		}
		if normalize(MapPath(server, filepath.Join(root, filepath.FromSlash(rel)))) != want {
			continue
		}
		if match.id != 0 {
			return playbackFile{}, nil
		}
		match = candidate
	}
	return match, rows.Err()
}

func bindPlaybackFile(ctx context.Context, q *sql.Tx, server domain.MediaServer, entry *PlaybackEntry) error {
	match, err := matchPlaybackFile(ctx, q, server, entry.SourcePart, entry.TitleID)
	if err == nil && match.id != 0 {
		entry.FileID, entry.TitleID, entry.VersionID = &match.id, &match.title, &match.version
	}
	return err
}
