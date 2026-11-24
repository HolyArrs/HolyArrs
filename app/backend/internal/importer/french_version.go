package importer

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// nfoLimit is how much of one .nfo is read; MediaInfo text is a few KB.
const nfoLimit = 256 << 10

// FrenchVersionOf reads the French dub (VFF, VFQ, VFI, VF2) of a
// downloaded file whose name does not tag it: first the release's .nfo
// (next to the file, or one folder up for a file in a subfolder), then
// the file's own audio tracks. "" when neither says.
func FrenchVersionOf(file string) string {
	dir := filepath.Dir(file)
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if v := nfoFrenchVersion(d); v != "" {
			return v
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	return TracksFrenchVersion(file)
}

// TracksFrenchVersion reads the French dub from a file's audio tracks.
func TracksFrenchVersion(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	tracks, err := mediafiles.ReadTracks(f, st.Size())
	if err != nil {
		return ""
	}
	var vs []string
	for _, t := range tracks {
		if t.Kind == "audio" {
			vs = append(vs, releasename.FrenchVersionOfTrack(t.Language, t.Name))
		}
	}
	return releasename.FrenchVersionOfTracks(vs...)
}

func nfoFrenchVersion(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".nfo") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(f, nfoLimit))
		f.Close()
		if v := releasename.FrenchVersionOfNFO(nfoText(b)); v != "" {
			return v
		}
	}
	return ""
}

// nfoText turns a UTF-16 .nfo (MediaInfo exports on Windows) into plain
// text; other encodings pass as they are, since the tags are ASCII.
func nfoText(b []byte) string {
	le := len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE
	be := len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF
	if !le && !be {
		return string(b)
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		if le {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		}
	}
	return string(utf16.Decode(u))
}
