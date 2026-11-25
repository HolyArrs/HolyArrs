package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A MULTi release whose name leaves out the dub gets it from its .nfo
// (next to the file or one folder up) when the pattern asks for it.
func TestFrenchVersionFromNFO544(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Movie.2024.MULTi.1080p.WEB-GRP")
	sub := filepath.Join(dir, "files")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "Movie.2024.MULTi.1080p.WEB-GRP.mkv")
	if err := os.WriteFile(file, []byte("not a real video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FrenchVersionOf(file); got != "" {
		t.Fatalf("no nfo = %q", got)
	}
	nfo := "Audio #1\nLanguage : English\n\nAudio #2\nTitle : French (CA)\nLanguage : French\n"
	if err := os.WriteFile(filepath.Join(dir, "grp.NFO"), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FrenchVersionOf(file); got != "VFQ" {
		t.Fatalf("nfo = %q", got)
	}

	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	ns.FilePattern = "{release}{.french_version}"
	tg := Target{TitleName: "Movie", Year: 2024, Naming: &ns}
	if got := tg.fileName(releasename.Parse("Movie.2024.MULTi.1080p.WEB-GRP", releasename.Hints{}), file); got != "Movie.2024.MULTi.1080p.WEB-GRP.VFQ.mkv" {
		t.Fatalf("file name = %q", got)
	}
	ns.FilePattern = "{release}"
	if got := tg.fileName(releasename.Parse("Movie.2024.MULTi.1080p.WEB-GRP", releasename.Hints{}), file); got != "Movie.2024.MULTi.1080p.WEB-GRP.mkv" {
		t.Fatalf("pattern without the word = %q", got)
	}
}

func TestNFOTextUTF16(t *testing.T) {
	src := "Audio\r\nLanguage : French (CA)\r\n"
	b := []byte{0xFF, 0xFE}
	for _, r := range src {
		b = append(b, byte(r), 0)
	}
	if got := releasename.FrenchVersionOfNFO(nfoText(b)); got != "VFQ" {
		t.Fatalf("utf-16 nfo: %q", got)
	}
}
