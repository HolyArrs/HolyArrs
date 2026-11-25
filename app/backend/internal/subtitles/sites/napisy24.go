package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// napisy24 finds Polish subtitles by the video file's OpenSubtitles-style hash
// and size through the site's CheckSubAgent endpoint, the one subliminal and
// Bazarr use. The answer is a status line and, when found, the subtitle as a
// zip in the same reply. It signs in with the owner's own Napisy24 account;
// HolyArrs ships no shared login, so a change on the site's side cannot break
// every install at once.
type napisy24 struct {
	c *client
}

const (
	n24Path = "/run/CheckSubAgent.php"
)

func newNapisy24(c *client) *napisy24 {
	c.keyHeader = ""
	return &napisy24{c: c}
}

// n24Key is the candidate key: the hash and the size, which is all a
// download needs to ask again.
func n24Key(hash string, size int64) string { return hash + "-" + strconv.FormatInt(size, 10) }

func n24Parse(key string) (string, int64, bool) {
	hash, sz, ok := strings.Cut(key, "-")
	size, err := strconv.ParseInt(sz, 10, 64)
	return hash, size, ok && err == nil && size > 0 && hex16.MatchString(hash)
}

// ask posts one check and returns the zip, or nil when the site has no
// subtitle of its own for the file.
func (n *napisy24) ask(ctx context.Context, hash string, size int64, name string) ([]byte, error) {
	if n.c.cfg.Username == "" || n.c.cfg.Password == "" {
		return nil, &Error{Kind: KindLoginRefused, Msg: "Napisy24 needs your user name and password"}
	}
	form := url.Values{}
	form.Set("postAction", "CheckSub")
	form.Set("ua", n.c.cfg.Username)
	form.Set("ap", n.c.cfg.Password)
	form.Set("fs", strconv.FormatInt(size, 10))
	form.Set("fh", hash)
	form.Set("fn", name)
	form.Set("n24pref", "1")
	raw, err := n.c.ref(n24Path)
	if err != nil {
		return nil, err
	}
	body, err := n.c.do(ctx, http.MethodPost, raw, []byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		"", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	head, rest, _ := bytes.Cut(body, []byte("||"))
	status := string(head)
	switch {
	case strings.HasPrefix(status, "login error"):
		return nil, &Error{Kind: KindLoginRefused, Msg: "the site refused the user name or password"}
	case !strings.HasPrefix(status, "OK-"):
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent an answer HolyArrs cannot read"}
	}
	// OK-0 is nothing, OK-1 only facts about the video, OK-3 a subtitle that is
	// not from the site's own collection; only OK-2 carries its subtitle.
	if !strings.HasPrefix(status, "OK-2") {
		return nil, nil
	}
	if len(rest) == 0 {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site said it found a subtitle but sent none"}
	}
	return rest, nil
}

// n24Text unzips the one subtitle and turns it into UTF-8 SRT. The site
// sends SRT by preference, but older entries can be in the frame or tenth-of-
// a-second formats NapiProjekt also uses.
func n24Text(z []byte) ([]byte, string, bool) {
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		return nil, "", false
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > maxDownloadBody {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, "", false
		}
		b, err := io.ReadAll(io.LimitReader(r, maxDownloadBody+1))
		_ = r.Close()
		if err != nil || len(b) > maxDownloadBody {
			return nil, "", false
		}
		return napiConvert(b)
	}
	return nil, "", false
}

func (n *napisy24) Search(ctx context.Context, q Query) ([]Candidate, error) {
	hash := strings.ToLower(q.Hash)
	if (q.Kind != "movie" && q.Kind != "episode") || !hex16.MatchString(hash) || q.Size <= 0 || !langWanted(wantedLangs(q.Languages), "pl") {
		return nil, nil
	}
	name := "video.mkv"
	if q.File != "" {
		name = filepath.Base(q.File)
	}
	z, err := n.ask(ctx, hash, q.Size, name)
	if err != nil || z == nil {
		return nil, err
	}
	_, format, ok := n24Text(z)
	if !ok {
		return nil, nil
	}
	cand := Candidate{Site: n.c.cfg.Name, Key: n24Key(hash, q.Size), Language: "pl", Title: q.Title, Year: q.Year, MatchedByID: true, Format: format,
		Link: name}
	if q.Kind == "episode" {
		cand.Season, cand.Episode = q.Season, q.Episode
	}
	if q.ReleaseName != "" {
		// The hash matches this exact file, so the subtitle is made for this release.
		cand.ReleaseNames = []string{q.ReleaseName}
	}
	return []Candidate{cand}, nil
}

func (n *napisy24) Download(ctx context.Context, c Candidate) ([]byte, error) {
	hash, size, ok := n24Parse(c.Key)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle has no file hash"}
	}
	name := c.Link
	if name == "" || strings.ContainsAny(name, `/\`) {
		name = "video.mkv"
	}
	z, err := n.ask(ctx, hash, size, name)
	if err != nil {
		return nil, err
	}
	if z == nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site no longer has this subtitle"}
	}
	out, _, ok := n24Text(z)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a subtitle in a format HolyArrs cannot read"}
	}
	return out, nil
}
