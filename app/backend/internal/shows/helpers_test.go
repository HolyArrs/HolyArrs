package shows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var (
	now  = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	dune = metadata.Movie{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"}, Runtime: 166,
		CinemaDate: d("2024-03-01"), DigitalDate: d("2024-04-16"), PhysicalDate: d("2024-05-14")}
)

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// adoptTMDB stands in for TMDB in the adopt tests.
var adoptTMDB = &metadata.Static{Movies: []metadata.Movie{
	dune,
	{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, DigitalDate: d("1996-01-01")},
	{Name: "The Lantern Keepers", Year: 2027, IDs: map[string]string{"tmdb": "900001"}},
	{Name: "The Lantern Keepers Return", Year: 2029, IDs: map[string]string{"tmdb": "900002"}},
	{Name: "Empty Movie", Year: 2001, IDs: map[string]string{"tmdb": "5001"}, DigitalDate: d("2001-05-01")},
}}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if strings.HasPrefix(e.Name(), ".") {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(to)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, s *database.Store, table string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// t10Golden compares against testdata/<name>; PIN_UPDATE=1 writes it.
func t10Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.WriteFile(p, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s missing: %v", p, err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("golden %s differs:\n%s", p, got)
	}
}

func d(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func countJobs(t *testing.T, s *database.Store, kind string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func slotsOf(t *testing.T, s *database.Store, titleID int64) []mediafiles.Slot {
	t.Helper()
	sl, err := database.Select[mediafiles.Slot](context.Background(), s.Read(), "slots", "item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", titleID)
	if err != nil {
		t.Fatal(err)
	}
	return sl
}

func waitJobs(t *testing.T, s *database.Store, n int) []domain.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "1=1 ORDER BY id")
		done := 0
		for _, j := range js {
			if j.State == "done" || j.State == "failed" {
				done++
			}
		}
		if len(js) >= n && done == len(js) {
			return js
		}
		time.Sleep(10 * time.Millisecond)
	}
	js, _ := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "1=1 ORDER BY id")
	t.Fatalf("jobs never settled: %+v", js)
	return nil
}

func writeInspectionMP4(t *testing.T, file string) {
	t.Helper()
	ftyp := inspectionMP4Box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	moov := inspectionMP4Box("moov", bytes.Join([][]byte{
		inspectionMP4Track("vide", "hvc1", "", "und"),
		inspectionMP4Track("soun", "mp4a", "fr-FR", "und"),
	}, nil))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(ftyp, moov...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// snapshot lists every file under root with its content hash.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		sum := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func byFolder(p library.AdoptPlan) map[string]library.AdoptCandidate {
	m := map[string]library.AdoptCandidate{}
	for _, c := range p.Folders {
		m[c.Folder] = c
	}
	return m
}

func fileRow(t *testing.T, s *database.Store, id int64) mediafiles.File {
	t.Helper()
	var f mediafiles.File
	if err := database.Get(context.Background(), s.Read(), "files", id, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func addFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := library.AddFolder(context.Background(), s, library.FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func onDisk(lf mediafiles.LibraryFolder, rel string) bool {
	_, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(rel)))
	return err == nil
}

func inspectionMP4Track(handler, codec, extendedLanguage, packedLanguage string) []byte {
	mdia := inspectionMP4Box("mdhd", func() []byte {
		payload := make([]byte, 24)
		if len(packedLanguage) == 3 {
			packed := uint16(packedLanguage[0]-0x60)<<10 | uint16(packedLanguage[1]-0x60)<<5 | uint16(packedLanguage[2]-0x60)
			binary.BigEndian.PutUint16(payload[20:22], packed)
		}
		return payload
	}())
	if extendedLanguage != "" {
		mdia = append(mdia, inspectionMP4Box("elng", append([]byte{0, 0, 0, 0}, append([]byte(extendedLanguage), 0)...))...)
	}
	hdlr := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte(handler)...)
	hdlr = append(hdlr, make([]byte, 12)...)
	mdia = append(mdia, inspectionMP4Box("hdlr", hdlr)...)
	entry := make([]byte, 16)
	binary.BigEndian.PutUint32(entry[:4], uint32(len(entry)))
	copy(entry[4:8], codec)
	stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
	mdia = append(mdia, inspectionMP4Box("minf", inspectionMP4Box("stbl", inspectionMP4Box("stsd", stsd)))...)
	return inspectionMP4Box("trak", inspectionMP4Box("mdia", mdia))
}

func inspectionMP4Box(kind string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], kind)
	copy(box[8:], payload)
	return box
}
