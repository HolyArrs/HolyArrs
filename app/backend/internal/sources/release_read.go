package sources

import (
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ReleaseRead is what the server read from a Release name, the same
// reading the judge used, so the screens show one truth.
type ReleaseRead struct {
	Resolution int      `json:"resolution,omitempty"` // 2160, 1080, 720, 480
	Source     string   `json:"source,omitempty"`     // bluray, remux, webdl, webrip, hdtv, dvd, ...
	Codec      string   `json:"codec,omitempty"`
	HDR        []string `json:"hdr,omitempty"`
	Audio      string   `json:"audio,omitempty"`
	Channels   string   `json:"channels,omitempty"`
	Atmos      bool     `json:"atmos,omitempty"`
	Languages  []string `json:"languages,omitempty"`
	Edition    []string `json:"edition,omitempty"`
	Group      string   `json:"group,omitempty"`
	Proper     bool     `json:"proper,omitempty"`
	Format     string   `json:"format,omitempty"` // music: flac, mp3, ...
	Bitrate    int      `json:"bitrate,omitempty"`
	BitDepth   int      `json:"bit_depth,omitempty"`
	Lossless   bool     `json:"lossless,omitempty"`
}

// readOf turns the stored reading back into its short view; nil when
// nothing was stored (an old Release) or nothing useful was read.
func readOf(parsed map[string]any) *ReleaseRead {
	if len(parsed) == 0 {
		return nil
	}
	b, err := json.Marshal(parsed)
	if err != nil {
		return nil
	}
	var p releasename.Result
	if json.Unmarshal(b, &p) != nil {
		return nil
	}
	r := &ReleaseRead{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value,
		Audio: p.Audio.Format, Channels: p.Audio.Channels, Atmos: p.Audio.Atmos, Languages: p.Languages, Edition: p.Edition,
		Group: p.Group.Value, Proper: p.Fix > 0}
	if m := p.Music; m != nil {
		r.Format, r.Bitrate, r.BitDepth, r.Lossless = m.Format, m.Bitrate, m.BitDepth, m.Lossless
	}
	if r.Resolution == 0 && r.Source == "" && r.Codec == "" && len(r.HDR) == 0 && r.Audio == "" && r.Format == "" && len(r.Languages) == 0 && r.Group == "" {
		return nil
	}
	return r
}
