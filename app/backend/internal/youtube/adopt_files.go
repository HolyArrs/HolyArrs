package youtube

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// videoExts are the video files adopting picks up.
var videoExts = map[string]bool{".mp4": true, ".mkv": true, ".webm": true, ".m4v": true}

var (
	// reNameID is a video id in square brackets, as yt-dlp names files.
	reNameID = regexp.MustCompile(`\[([A-Za-z0-9_-]{11})\]`)
	// reNameDay is an upload date at the start of a name.
	reNameDay = regexp.MustCompile(`^(\d{4})-?(\d{2})-?(\d{2}) +`)
	// reNameIndex is Pinchflat's "NN - " at the start of a name.
	reNameIndex = regexp.MustCompile(`^\d{1,4} - `)
	// reSubLang is the language part of "<name>.<lang>.srt".
	reSubLang = regexp.MustCompile(`^[A-Za-z0-9_-]{1,35}$`)
	// reDuration is the length ffmpeg prints about its input.
	reDuration = regexp.MustCompile(`Duration: (\d+):(\d{2}):(\d{2}(?:\.\d+)?)`)
)

// ffmpegTimeout bounds one ffmpeg run of adopting.
const ffmpegTimeout = 2 * time.Minute

// adoptSource is one video file found and what its files say about it.
type adoptSource struct {
	item AdoptItem
	name string // slash path under the source folder
	size int64
	meta Meta
	// again is set for a second copy of a video in a folder given a
	// playlist: it is not copied, the first copy joins that playlist.
	again bool
}

// target is the source's path under the person's folder: in folder, else
// its channel's folder, else one named like the folder it came from.
func (a adoptSource) target(m Meta, folder string) string {
	if folder == "" {
		folder = m.ChannelName
	}
	if folder == "" {
		folder = a.item.Folder
	}
	return SafeName(folder, NameBytes) + "/" + VideoBase(m.UploadedAt, m.Title, a.item.VideoID) + path.Ext(a.name)
}

// scanAdopt adds every video file under the source folder, read through
// dir, which resolves every name inside the folder: no symlink or swapped
// folder leads outside it. top names the folder itself. Hidden entries and
// symlinks are left alone.
func scanAdopt(dir *os.Root, top string, out *[]adoptSource) error {
	return fs.WalkDir(dir.FS(), ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if strings.HasPrefix(e.Name(), ".") {
			if e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() || !videoExts[strings.ToLower(path.Ext(name))] {
			return nil
		}
		src, err := readSource(dir, name)
		if errors.Is(err, errNotPlain) {
			return nil
		}
		if err != nil {
			return err
		}
		src.item.From, src.item.Folder = name, top
		if d := path.Dir(name); d != "." {
			src.item.Folder = path.Base(d)
		}
		*out = append(*out, src)
		return nil
	})
}

// readSource reads what a video's files say: its .nfo (Pinchflat, Kodi),
// then its .info.json (yt-dlp), then its name; the file's time is the
// upload date of last resort.
func readSource(dir *os.Root, name string) (adoptSource, error) {
	st, err := dir.Lstat(name)
	if err != nil {
		return adoptSource{}, err
	}
	if !st.Mode().IsRegular() {
		return adoptSource{}, errNotPlain
	}
	stem := strings.TrimSuffix(name, path.Ext(name))
	src := adoptSource{name: name, size: st.Size()}
	m := &src.meta
	if n, ok := readNFO(dir, stem+".nfo"); ok {
		src.item.VideoID = n.youtubeID()
		m.Title, m.ChannelName = strings.TrimSpace(n.Title), strings.TrimSpace(n.ShowTitle)
		m.UploadedAt = parseDay(n.Aired)
		if m.UploadedAt == nil {
			m.UploadedAt = parseDay(n.Premiered)
		}
	}
	if raw, ok := readInfoJSON(dir, stem+".info.json"); ok {
		if src.item.VideoID == "" && reVideoID.MatchString(raw.ID) {
			src.item.VideoID = raw.ID
		}
		m.Title = or(m.Title, raw.Title)
		m.ChannelName = or(m.ChannelName, channelName(raw))
		m.ChannelID = or(m.ChannelID, raw.ChannelID)
		if m.UploadedAt == nil {
			m.UploadedAt = uploadedAt(raw)
		}
		m.DurationSeconds = seconds(raw.Duration)
	}
	title, day, id := parseName(path.Base(stem))
	if src.item.VideoID == "" {
		src.item.VideoID = id
	}
	m.Title = or(m.Title, title)
	if m.UploadedAt == nil {
		m.UploadedAt = day
	}
	if m.UploadedAt == nil {
		t := st.ModTime().UTC()
		m.UploadedAt = &t
	}
	return src, nil
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// nfoDoc is the part of a Kodi .nfo adopting reads.
type nfoDoc struct {
	Title     string `xml:"title"`
	ShowTitle string `xml:"showtitle"`
	Aired     string `xml:"aired"`
	Premiered string `xml:"premiered"`
	IDs       []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
}

func (n nfoDoc) youtubeID() string {
	for _, id := range n.IDs {
		if v := strings.TrimSpace(id.Value); strings.EqualFold(id.Type, "youtube") && reVideoID.MatchString(v) {
			return v
		}
	}
	return ""
}

// readNFO reads an .nfo; false when there is none or it is not XML.
func readNFO(dir *os.Root, name string) (nfoDoc, bool) {
	b, err := readSmall(dir, name)
	if err != nil {
		return nfoDoc{}, false
	}
	var n nfoDoc
	return n, xml.Unmarshal(b, &n) == nil
}

// readInfoJSON reads a yt-dlp .info.json; false when there is none or it
// is not JSON.
func readInfoJSON(dir *os.Root, name string) (rawInfo, bool) {
	b, err := readSmall(dir, name)
	if err != nil {
		return rawInfo{}, false
	}
	var raw rawInfo
	return raw, json.Unmarshal(b, &raw) == nil
}

// readSmall reads a side file of up to 32 MiB.
func readSmall(dir *os.Root, name string) ([]byte, error) {
	f, _, err := openPlain(dir, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 32<<20))
}

// parseDay reads the YYYY-MM-DD date at the start of s, as an .nfo writes it.
func parseDay(s string) *time.Time {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return nil
	}
	return &t
}

// parseName reads a title, an upload date and a video id from a file name
// without its extension: "YYYY-MM-DD Title [id]", "NN - Title" or "Title".
func parseName(stem string) (title string, day *time.Time, id string) {
	title = stem
	if all := reNameID.FindAllStringSubmatchIndex(title, -1); len(all) > 0 {
		m := all[len(all)-1]
		id = title[m[2]:m[3]]
		title = title[:m[0]] + title[m[1]:]
	}
	title = strings.TrimSpace(title)
	if m := reNameDay.FindStringSubmatch(title); m != nil {
		if t, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]); err == nil {
			day = &t
			title = title[len(m[0]):]
		}
	}
	title = strings.TrimSpace(reNameIndex.ReplaceAllString(title, ""))
	return title, day, id
}

// copyFile copies from, in the source folder, to to through
// "<to>.partial", checking the size before the rename. Errors carry no
// path: paths hold titles.
func copyFile(ctx context.Context, dir *os.Root, from, to string) error {
	in, st, err := openPlain(dir, from)
	if err != nil {
		return pathFree(err)
	}
	defer in.Close()
	tmp := to + ".partial"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return pathFree(err)
	}
	n, err := io.Copy(out, ctxReader{ctx, in})
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != st.Size() {
		err = errors.New("a copied video came out a different size")
	}
	if err == nil {
		err = os.Rename(tmp, to)
	}
	if err != nil {
		os.Remove(tmp)
		return pathFree(err)
	}
	return nil
}

// ctxReader stops a copy once its context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// adoptExtras gives an adopted video file its thumbnail and subtitles and
// returns the files made. The thumbnail is the source's .jpg or -thumb.jpg,
// else its .webp or .png turned into a .jpg, else a frame at 10 % of the
// copy. Subtitles "<name>.<lang>.srt" keep their language. Neither is
// needed: a failure leaves it out.
func adoptExtras(ctx context.Context, ffmpeg string, dir *os.Root, src adoptSource, file string, duration int) []string {
	var made []string
	if thumb := thumbOf(file); adoptThumb(ctx, ffmpeg, dir, src, file, thumb, duration) == nil {
		made = append(made, thumb)
	}
	stem := strings.TrimSuffix(file, filepath.Ext(file))
	for lang, sub := range subtitlesOf(dir, src.name) {
		to := stem + "." + lang + ".srt"
		if copyFile(ctx, dir, sub, to) == nil {
			made = append(made, to)
		}
	}
	return made
}

// adoptThumb makes dst for the copy file of src. ffmpeg never opens a
// source file by path: a still image reaches it on stdin and the frame
// comes from the copy HolyArrs just wrote.
func adoptThumb(ctx context.Context, ffmpeg string, dir *os.Root, src adoptSource, file, dst string, duration int) error {
	stem := strings.TrimSuffix(src.name, path.Ext(src.name))
	for _, c := range []string{".jpg", "-thumb.jpg", ".jpeg", "-thumb.jpeg"} {
		if regular(dir, stem+c) {
			return copyFile(ctx, dir, stem+c, dst)
		}
	}
	if ffmpeg == "" {
		return errors.New("no ffmpeg")
	}
	for _, c := range []string{".webp", "-thumb.webp", ".png", "-thumb.png"} {
		if !regular(dir, stem+c) {
			continue
		}
		f, _, err := openPlain(dir, stem+c)
		if err != nil {
			return err
		}
		defer f.Close()
		return ffmpegJPEG(ctx, ffmpeg, dst, f, "-i", "pipe:0")
	}
	at := strconv.FormatFloat(float64(duration)/10, 'f', 2, 64)
	return ffmpegJPEG(ctx, ffmpeg, dst, nil, "-ss", at, "-i", file)
}

// regular reports whether name in the source folder is a plain file: a
// symlink is not, so nothing outside the folder is read through one.
func regular(dir *os.Root, name string) bool {
	st, err := dir.Lstat(name)
	return err == nil && st.Mode().IsRegular()
}

// errNotPlain refuses a source file that is a symlink or not a file.
var errNotPlain = errors.New("not a plain file")

// openPlain opens a file in the source folder only when it is a plain file
// and not a symlink, and checks that what was opened is what was looked at.
func openPlain(dir *os.Root, name string) (*os.File, os.FileInfo, error) {
	before, err := dir.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, errNotPlain
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !os.SameFile(before, st) {
		f.Close()
		if err == nil {
			err = errNotPlain
		}
		return nil, nil, err
	}
	return f, st, nil
}

// ffmpegJPEG writes the first frame of the input as a .jpg at dst. stdin,
// when set, is what ffmpeg reads for "-i pipe:0".
func ffmpegJPEG(ctx context.Context, ffmpeg, dst string, stdin *os.File, input ...string) error {
	ctx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()
	tmp := dst + ".partial"
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if stdin == nil {
		args = append([]string{"-nostdin"}, args...)
	}
	args = append(append(args, input...), "-frames:v", "1", "-c:v", "mjpeg", "-f", "image2", "-update", "1", tmp)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if err := cmd.Run(); err != nil {
		os.Remove(tmp)
		return err
	}
	if st, err := os.Lstat(tmp); err != nil || !st.Mode().IsRegular() {
		os.Remove(tmp)
		return errors.New("ffmpeg made no thumbnail")
	}
	return os.Rename(tmp, dst)
}

// videoSeconds is a video's length as ffmpeg reads it, 0 when unknown.
func videoSeconds(ctx context.Context, ffmpeg, file string) int {
	if ffmpeg == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()
	// With no output ffmpeg exits non-zero after printing what it read.
	out, _ := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-i", file).CombinedOutput()
	m := reDuration.FindSubmatch(out)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(string(m[1]))
	mins, _ := strconv.Atoi(string(m[2]))
	sec, _ := strconv.ParseFloat(string(m[3]), 64)
	return int(math.Round(float64(h*3600+mins*60) + sec))
}

// subtitlesOf finds the subtitles next to a video file in the source
// folder, "<name>.<lang>.srt", by language.
func subtitlesOf(dir *os.Root, name string) map[string]string {
	folder, base := path.Split(name)
	stem := strings.TrimSuffix(base, path.Ext(base)) + "."
	entries, err := fs.ReadDir(dir.FS(), path.Clean(or(folder, ".")))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, stem) || !strings.HasSuffix(name, ".srt") {
			continue
		}
		if lang := strings.TrimSuffix(name[len(stem):], ".srt"); reSubLang.MatchString(lang) {
			out[lang] = path.Join(folder, name)
		}
	}
	return out
}
