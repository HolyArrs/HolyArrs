package importer

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Side files: subtitle files that came with a download are brought into the
// Library beside their video on import, when that is turned on (off by
// default, srt only). A file already in the Library is never replaced.

// SideFiles is the side files setting.
type SideFiles struct {
	On    bool     `json:"on"`
	Types []string `json:"types"`
}

const sideGroup, sideOnKey, sideTypesKey = "where_files_go", "side_files", "side_file_types"

// ErrSideFileTypes is returned for a bad list of side file types.
var ErrSideFileTypes = errors.New("list 1 to 10 file types, such as srt or ass, each 1 to 8 letters or digits")

// languageOf turns a language word or code into its code ("" when unknown).
// The subtitles module hands its own over at startup with UseLanguages.
var languageOf = func(string) string { return "" }

// UseLanguages sets how side file names are read for their language.
func UseLanguages(f func(string) string) {
	if f != nil {
		languageOf = f
	}
}

// ReadSideFiles loads the side files setting; off with srt when never saved.
func ReadSideFiles(ctx context.Context, db database.Lister) SideFiles {
	out := SideFiles{Types: []string{"srt"}}
	_ = database.EachSetting(ctx, db, sideGroup, func(k, v string) {
		switch k {
		case sideOnKey:
			out.On = v == "true"
		case sideTypesKey:
			if ts, err := cleanSideTypes(strings.Split(v, ",")); err == nil {
				out.Types = ts
			}
		}
	})
	return out
}

// SaveSideFiles checks and stores the side files setting.
func SaveSideFiles(ctx context.Context, s *database.Store, in SideFiles, by domain.Actor) (SideFiles, error) {
	ts, err := cleanSideTypes(in.Types)
	if err != nil {
		return SideFiles{}, err
	}
	in.Types = ts
	err = s.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now()
		if err := database.PutSetting(tx, sideGroup, sideOnKey, strconv.FormatBool(in.On), by, now); err != nil {
			return err
		}
		return database.PutSetting(tx, sideGroup, sideTypesKey, strings.Join(ts, ","), by, now)
	})
	return in, err
}

func cleanSideTypes(in []string) ([]string, error) {
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimLeft(strings.TrimSpace(t), "."))
		if t == "" {
			continue
		}
		if len(t) > 8 || strings.ContainsFunc(t, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) || IsVideo("."+t) {
			return nil, ErrSideFileTypes
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) == 0 || len(out) > 10 {
		return nil, ErrSideFileTypes
	}
	return out, nil
}

// sideFile is one side file and where it goes.
type sideFile struct{ src, target string }

// sideFilesFor picks the side files of video among candidates and names each
// beside target. A side file is the video's when (1) its name starts with the
// video's name, or it sits in a folder named after the video; (2) for a show,
// its name has the same episode numbers; or (3) onlyVideo: the download has a
// single real video. So a pack whose subtitles are named only by language
// gets none. Each name is the video's new name, the language code and tags
// read from the side file, then a copy number when two would collide.
func sideFilesFor(video, target string, candidates []string, onlyVideo, show bool, types []string) []sideFile {
	stem := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	tstem := strings.TrimSuffix(target, filepath.Ext(target))
	var eps string
	if show {
		eps = episodeKey(filepath.Base(video))
	}
	cands := slices.Clone(candidates)
	slices.Sort(cands)
	used := map[string]bool{}
	var out []sideFile
	for _, c := range cands {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(c), "."))
		if !slices.Contains(types, ext) {
			continue
		}
		cstem := strings.TrimSuffix(filepath.Base(c), filepath.Ext(c))
		var tags string
		switch {
		case strings.EqualFold(cstem, stem):
		case len(cstem) > len(stem) && strings.EqualFold(cstem[:len(stem)+1], stem+"."):
			tags = cstem[len(stem)+1:]
		case filepath.Dir(c) != filepath.Dir(video) && strings.EqualFold(filepath.Base(filepath.Dir(c)), stem):
			tags = cstem
		case eps != "" && episodeKey(filepath.Base(c)) == eps:
			tags = cstem
		case onlyVideo:
			tags = cstem
		default:
			continue
		}
		suffix := sideSuffix(tags)
		name := tstem + suffix + "." + ext
		for n := 1; used[strings.ToLower(name)]; n++ {
			name = tstem + suffix + "." + strconv.Itoa(n) + "." + ext
		}
		used[strings.ToLower(name)] = true
		out = append(out, sideFile{src: c, target: name})
	}
	return out
}

// episodeKey is the season and episode numbers a name carries, or "".
func episodeKey(name string) string {
	p := releasename.Parse(name, releasename.Hints{})
	if p.Show == nil || len(p.Show.Episodes) == 0 {
		return ""
	}
	parts := []string{}
	for _, s := range p.Show.Seasons {
		parts = append(parts, "s"+strconv.Itoa(s))
	}
	for _, e := range p.Show.Episodes {
		parts = append(parts, "e"+strconv.Itoa(e))
	}
	return strings.Join(parts, "")
}

// sideSuffix reads a language and the forced or hearing impaired tags from
// the words of a side file name: ".en", ".en.forced", ".en.hi" or "".
func sideSuffix(tags string) string {
	words := strings.FieldsFunc(strings.ToLower(tags), func(r rune) bool {
		return r == '.' || r == '_' || r == ' ' || r == '[' || r == ']' || r == '(' || r == ')'
	})
	lang, forced, hi := "", false, false
	for _, w := range words {
		switch w {
		case "forced", "foreign":
			forced = true
			continue
		case "hi", "cc", "sdh":
			hi = true
			continue
		}
		if lang == "" {
			lang = languageOf(w)
		}
	}
	if lang == "" {
		return ""
	}
	s := "." + lang
	if hi {
		s += ".hi"
	}
	if forced {
		s += ".forced"
	}
	return s
}

// sideCandidates lists the files beside a video and in Subs folders under its
// folder, and how many real and sample videos that folder holds.
func sideCandidates(dir string) (files []string, real, all int) {
	var walk func(d string, depth int, inSubs bool)
	walk = func(d string, depth int, inSubs bool) {
		ents, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range ents {
			p := filepath.Join(d, e.Name())
			switch {
			case e.IsDir():
				low := strings.ToLower(e.Name())
				if depth < 2 && (inSubs || low == "subs" || low == "subtitles") {
					walk(p, depth+1, true)
				}
			case e.Type().IsRegular():
				if IsVideo(filepath.Ext(p)) {
					all++
					if !isSample(p, releasename.Parse(e.Name(), releasename.Hints{})) {
						real++
					}
					continue
				}
				files = append(files, p)
			}
		}
	}
	walk(dir, 0, false)
	return files, real, all
}

// placeSideFiles brings the side files of each placed video along, linked
// or copied as the video was. from is what the import read: only a folder
// download may take every side file for its single video. It never replaces
// a file already in the Library and never fails the import.
func (im *Importer) placeSideFiles(ctx context.Context, from string, plan []mediafiles.PlanStep, res []mediafiles.PlaceResult, show bool) {
	cfg := ReadSideFiles(ctx, im.Store.Read())
	if !cfg.On {
		return
	}
	fi, err := os.Stat(from)
	folder := err == nil && fi.IsDir()
	ops := im.Ops
	if ops.Link == nil {
		ops.Link = os.Link
	}
	if ops.Copy == nil {
		ops.Copy = copyFile
	}
	ops.Move = nil
	for i, s := range plan {
		if i >= len(res) || res[i].Outcome != "placed" || s.TargetPath == "" {
			continue
		}
		cands, real, all := sideCandidates(filepath.Dir(s.SourceFile))
		for _, sf := range sideFilesFor(s.SourceFile, s.TargetPath, cands, folder && real == 1 && all <= 2, show, cfg.Types) {
			placeSideFile(sf, ops)
		}
	}
}

// placeSideFile places one side file, keeping any file already at its target.
func placeSideFile(sf sideFile, ops Ops) {
	if _, err := os.Lstat(sf.target); err == nil {
		log.Printf("importer: kept %s already in the Library; the side file %s was not brought in", sf.target, sf.src)
		return
	}
	r := place(mediafiles.PlanStep{SourceFile: sf.src, TargetPath: sf.target, Method: "hardlink"}, ops, time.Time{})
	if r.Outcome != "placed" {
		log.Printf("importer: side file %s not brought in: %s", sf.src, r.Error)
	}
}
