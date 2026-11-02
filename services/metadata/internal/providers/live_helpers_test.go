//go:build metadata_live

package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiveReportRefusesOverwriteAndCredentialPath(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "tmdb-token")
	if err := os.WriteFile(token, []byte("synthetic-test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, _, err := openLiveReport(token, token); err == nil {
		file.Close()
		t.Fatal("report accepted credential path")
	}
	report := filepath.Join(dir, "report.json")
	file, _, err := openLiveReport(report, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveLiveReport(file, map[string]any{"passed": true}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if file, _, err := openLiveReport(report, token); err == nil {
		file.Close()
		t.Fatal("existing report was accepted")
	}
	after, err := os.ReadFile(report)
	if err != nil || string(after) != string(before) {
		t.Fatal("existing report changed")
	}
}

func TestLiveFieldContractsRejectMissingUsefulData(t *testing.T) {
	detail := map[string]any{
		"source": "tmdb", "source_id": "550", "kind": "movie", "title": "Example",
		"source_data": map[string]any{"id": float64(550), "title": "Example", "release_date": "1999-10-15"},
	}
	required := map[string]string{"id": "number", "title": "string", "release_date": "string"}
	if !liveDetailContract(detail, "tmdb", "movie", "detail", "550", 0, required) {
		t.Fatal("valid detail contract rejected")
	}
	detail["title"] = ""
	if liveDetailContract(detail, "tmdb", "movie", "detail", "550", 0, required) {
		t.Fatal("empty detail title accepted")
	}
	search := map[string]any{"items": []any{}}
	if liveSearchContract(search, "tvmaze", "show", map[string]string{"id": "number", "name": "string"}) {
		t.Fatal("empty stable search accepted")
	}
}
