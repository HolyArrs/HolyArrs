package downloads

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fileNames(fs []File) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

// A client that calls the content path a folder marks it with a trailing slash
// and then names the files below that folder without it. rdt-client answers
// this way for every torrent, a single file included, where the torrent folder
// and the file in it share a name. Reading those names as already relative to
// the save path leaves the folder itself for the import.
func TestFolderContentPathPrefixesBareNames(t *testing.T) {
	cases := []struct {
		name, save, content string
		in, want            []string
	}{
		{"single file in its own folder", "/downloads/complete/holyarrs",
			"/downloads/complete/holyarrs/Example.2026.2160p.WEB.H265-GRP.mkv/",
			[]string{"Example.2026.2160p.WEB.H265-GRP.mkv"},
			[]string{"Example.2026.2160p.WEB.H265-GRP.mkv/Example.2026.2160p.WEB.H265-GRP.mkv"}},
		{"multi file folder", "/d/t", "/d/t/Show.S01/", []string{"e1.mkv", "Subs/en.srt"},
			[]string{"Show.S01/e1.mkv", "Show.S01/Subs/en.srt"}},
		{"names that already carry the folder", "/d/t", "/d/t/Show.S01/",
			[]string{"Show.S01/e1.mkv", "Show.S01/Subs/en.srt"},
			[]string{"Show.S01/e1.mkv", "Show.S01/Subs/en.srt"}},
		{"rooted names under a folder", "/d/t", "/d/t/Show.S01/", []string{"/e1.mkv", "Subs/en.srt"},
			[]string{"Show.S01/e1.mkv", "Show.S01/Subs/en.srt"}},
		{"content path is one file", "/d/t", "/d/t/a.mkv", []string{"a.mkv"}, []string{"a.mkv"}},
		{"save folder itself", "/d/t", "/d/t/", []string{"a.mkv"}, []string{"a.mkv"}},
		{"folder outside the save path", "/d/t", "/etc/x/", []string{"a.mkv"}, []string{"a.mkv"}},
		{"no content path", "/d/t", "", []string{"a.mkv"}, []string{"a.mkv"}},
		{"windows folder", `D:\t`, `D:\t\M\`, []string{"x.mkv"}, []string{"M/x.mkv"}},
		{"climbing name kept for refusal", "/d/t", "/d/t/M/", []string{"../x.mkv"}, []string{"M/../x.mkv"}},
	}
	for _, c := range cases {
		var in []File
		for _, n := range c.in {
			in = append(in, File{Name: n})
		}
		if got := fileNames(rootedFileNames(in, c.save, c.content)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The finished file of a single-file torrent sits one folder below the save
// path when the client keeps the torrent folder. Read as if it were in the save
// path, the name finds that folder instead, whose own size is a few KB, and the
// import is refused as a fake.
func TestQbitBareNameUnderAContentFolderFindsTheFile(t *testing.T) {
	movie := "Example.2026.2160p.WEB.H265-GRP.mkv"
	save := filepath.Join(t.TempDir(), "holyarrs")
	folder := filepath.Join(save, movie)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("make the torrent folder: %v", err)
	}
	const size = 12 << 20
	if err := os.WriteFile(filepath.Join(folder, movie), make([]byte, size), 0o644); err != nil {
		t.Fatalf("write the movie: %v", err)
	}
	const hash = "a7c04066a7c04066a7c04066a7c04066a7c04066"
	content := filepath.Join(save, movie) + string(filepath.Separator)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			fmt.Fprintf(w, `[{"hash":%q,"name":%q,"category":"holyarrs","state":"pausedUP","progress":1,"size":%d,`+
				`"save_path":%q,"content_path":%q,"completion_on":1794951961}]`, hash, movie, size, save, content)
		case "/api/v2/torrents/files":
			fmt.Fprintf(w, `[{"name":%q,"size":%d,"progress":1}]`, movie, size)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	j, err := NewQbittorrentKey(srv.URL, "a-key", srv.Client()).Get(context.Background(), hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := movie + "/" + movie; len(j.Files) != 1 || j.Files[0].Name != want {
		t.Fatalf("files %q, want one named %q", fileNames(j.Files), want)
	}
	// What the import would open: it joins the save path and the file name.
	got := filepath.Join(j.SavePath, filepath.FromSlash(j.Files[0].Name))
	fi, err := os.Stat(got)
	if err != nil {
		t.Fatalf("the import would read %s: %v", got, err)
	}
	if fi.IsDir() || fi.Size() != size {
		t.Errorf("the import would read %s, size %d, a folder: %v", got, fi.Size(), fi.IsDir())
	}
}

// A multi-file torrent from the same client keeps every file in its folder, and
// a name that already carries the folder is not prefixed twice.
func TestQbitBareNamesOfAPackStayInTheirFolder(t *testing.T) {
	const hash = "b7c04066b7c04066b7c04066b7c04066b7c04066"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			fmt.Fprintf(w, `[{"hash":%q,"name":"Show.S01.1080p","category":"holyarrs","state":"pausedUP","progress":1,`+
				`"save_path":"/downloads/holyarrs","content_path":"/downloads/holyarrs/Show.S01.1080p/"}]`, hash)
		case "/api/v2/torrents/files":
			_, _ = w.Write([]byte(`[{"name":"Show.S01E01.mkv"},{"name":"Subs/en.srt"},{"name":"Show.S01.1080p/Show.S01E02.mkv"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	j, err := NewQbittorrentKey(srv.URL, "a-key", srv.Client()).Get(context.Background(), hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []string{"Show.S01.1080p/Show.S01E01.mkv", "Show.S01.1080p/Subs/en.srt", "Show.S01.1080p/Show.S01E02.mkv"}
	if got := fileNames(j.Files); !reflect.DeepEqual(got, want) {
		t.Errorf("files %q, want %q", got, want)
	}
	if strings.Contains(j.Files[0].Name, "Show.S01.1080p/Show.S01.1080p") {
		t.Errorf("the folder was added twice: %q", j.Files[0].Name)
	}
}
