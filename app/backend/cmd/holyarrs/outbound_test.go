package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// outsideHosts are the sources only the metadata server may call: an
// install asks the server, which asks once and keeps the answer.
var outsideHosts = []string{"api.deezer.com", "wikipedia.org", "musicbrainz.org", "api.tvmaze.com", "coverartarchive.org"}

// standInFiles are the direct clients, kept only as a developer stand-in
// (HOLYARRS_DIRECT_SOURCES=1, or a stand-in address in tests) and off for
// installs; TestDirectSourcesOffByDefault checks that they are off.
var standInFiles = map[string]bool{
	"internal/metadata/musicbrainz.go":   true, // MusicBrainz (stand-in, Settings > Music URL)
	"internal/metadata/tvmaze.go":        true, // TVmaze
	"internal/metadata/artist_images.go": true, // Deezer
	"internal/metadata/deezer_artist.go": true, // Deezer and Wikipedia bios
	// Only checks that a bio link the server sent is a Wikipedia page; it
	// makes no request.
	"internal/metadata/server_wikidata.go": true,
	// The host list per mode for firewalls (Settings > Metadata); it makes
	// no request.
	"internal/metadata/mode_table.go": true,
}

// serverOnlyHosts are the sources only the metadata server ever asks, in
// every mode: no direct client exists for them.
var serverOnlyHosts = []string{"apple.com", "spotify81", "kworb", "movieofthenight", "rapidapi"}

// TestServerOnlySourcesHaveNoClient: no Go file names a server-only
// source's host, so no mode can build a client for one.
func TestServerOnlySourcesHaveNoClient(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					v, _ := strconv.Unquote(lit.Value)
					for _, h := range serverOnlyHosts {
						if strings.Contains(strings.ToLower(v), h) {
							found = append(found, path+": "+v)
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) > 0 {
		t.Fatalf("server-only sources named in the app:\n%s", strings.Join(found, "\n"))
	}
}

// TestNoOutsideHostsOutsideStandIns: no Go file outside the stand-in
// clients names an outside source's host in a string, so nothing else can
// build a request to one.
func TestNoOutsideHostsOutsideStandIns(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if standInFiles[rel] {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				v, _ := strconv.Unquote(lit.Value)
				for _, h := range outsideHosts {
					if strings.Contains(strings.ToLower(v), h) {
						found = append(found, rel+": "+v)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) > 0 {
		t.Fatalf("outside hosts named outside the stand-in clients:\n%s", strings.Join(found, "\n"))
	}
}

// TestDirectSourcesOffByDefault: with no stand-in named, the install's
// Deezer, MusicBrainz and TVmaze clients are the metadata server's.
func TestDirectSourcesOffByDefault(t *testing.T) {
	for _, k := range []string{"HOLYARRS_DIRECT_SOURCES", "HOLYARRS_DEEZER_URL", "HOLYARRS_MUSICBRAINZ_URL", "HOLYARRS_TVMAZE_URL", metadata.EnvServiceURL, metadata.EnvTMDBKey, metadata.EnvTokenFile, "HOLYARRS_TMDB_URL"} {
		t.Setenv(k, "")
	}
	if directSources() || artistImages() != nil {
		t.Fatal("direct Deezer client on by default")
	}
	h, err := metadata.NewHosted(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, direct := showSources(nil, h)["tvmaze"].(*metadata.TVMaze); direct {
		t.Fatal("TVmaze asked directly by default")
	}
	// Developers only: the direct clients come back.
	t.Setenv("HOLYARRS_DIRECT_SOURCES", "1")
	if d := artistImages(); d == nil || d.BaseURL != metadata.DeezerBase {
		t.Fatalf("developer stand-in %+v", d)
	}
	if _, direct := showSources(nil, h)["tvmaze"].(*metadata.TVMaze); direct {
		t.Fatal("TVmaze not behind the server with the developer stand-in")
	}
	// A TVmaze stand-in (tests) is asked directly.
	t.Setenv("HOLYARRS_TVMAZE_URL", "http://127.0.0.1:18751")
	if _, direct := showSources(nil, h)["tvmaze"].(*metadata.TVMaze); !direct {
		t.Fatal("TVmaze stand-in not used")
	}
}
