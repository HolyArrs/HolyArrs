//go:build metadata_live

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

func sameLivePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func resolvedLivePath(path string) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("live path is unavailable or linked")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || !sameLivePath(abs, resolved) {
		return "", nil, fmt.Errorf("live path traverses a link or reparse point")
	}
	return abs, info, nil
}

func openLiveReport(reportPath, tokenPath string) (*os.File, string, error) {
	token, tokenInfo, err := resolvedLivePath(tokenPath)
	if err != nil || !tokenInfo.Mode().IsRegular() || tokenInfo.Size() < 1 || tokenInfo.Size() > 16384 {
		return nil, "", fmt.Errorf("live credential path is not a bounded regular file")
	}
	// Windows mode bits do not prove ACL privacy. The operator must separately
	// verify a restricted token file and ancestor ACL before opting in there.
	if runtime.GOOS != "windows" && tokenInfo.Mode().Perm()&0077 != 0 {
		return nil, "", fmt.Errorf("live credential permissions are not private")
	}
	report, err := filepath.Abs(reportPath)
	if err != nil || filepath.Base(report) == "." || filepath.Base(report) == string(filepath.Separator) {
		return nil, "", fmt.Errorf("invalid live report path")
	}
	parent, parentInfo, err := resolvedLivePath(filepath.Dir(report))
	if err != nil || !parentInfo.IsDir() {
		return nil, "", fmt.Errorf("live report parent is not a real directory")
	}
	report = filepath.Join(parent, filepath.Base(report))
	if sameLivePath(report, token) {
		return nil, "", fmt.Errorf("live report and credential paths must differ")
	}
	file, err := os.OpenFile(report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, "", fmt.Errorf("live report path must be new")
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		file.Close()
		_ = os.Remove(report)
		return nil, "", fmt.Errorf("live report is not a regular file")
	}
	return file, token, nil
}

func saveLiveReport(file *os.File, report map[string]any) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		data = append(data, '\n')
		var written int
		written, err = file.Write(data)
		if err == nil && written != len(data) {
			err = fmt.Errorf("short live report write")
		}
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file.Name())
	}
	return err
}

func liveField(value any, shape string) bool {
	switch shape {
	case "number":
		_, ok := value.(float64)
		return ok
	case "string":
		value, ok := value.(string)
		return ok && strings.TrimSpace(value) != ""
	case "array":
		value, ok := value.([]any)
		return ok && len(value) != 0
	case "object":
		value, ok := value.(map[string]any)
		return ok && len(value) != 0
	default:
		return false
	}
}

func liveSourceFields(source map[string]any, required map[string]string) bool {
	for name, shape := range required {
		if !liveField(source[name], shape) {
			return false
		}
	}
	return true
}

func liveDetailContract(data map[string]any, provider, kind, operation, id string, season int, required map[string]string) bool {
	if data["source"] != provider || data["kind"] != kind {
		return false
	}
	sourceID, idOK := data["source_id"].(string)
	title, titleOK := data["title"].(string)
	source, sourceOK := data["source_data"].(map[string]any)
	if !idOK || sourceID == "" || !titleOK || strings.TrimSpace(title) == "" || !sourceOK || !liveSourceFields(source, required) {
		return false
	}
	if operation == "detail" && !strings.EqualFold(sourceID, id) {
		return false
	}
	if operation == "season" {
		number, ok := source["season_number"].(float64)
		if !ok || number != float64(season) {
			return false
		}
	}
	return true
}

func liveSearchContract(data map[string]any, provider, kind string, required map[string]string) bool {
	items, ok := data["items"].([]any)
	if !ok || len(items) == 0 {
		return false
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["source"] != provider || item["kind"] != kind {
		return false
	}
	sourceID, idOK := item["source_id"].(string)
	title, titleOK := item["title"].(string)
	source, sourceOK := item["source_data"].(map[string]any)
	return idOK && sourceID != "" && titleOK && strings.TrimSpace(title) != "" && sourceOK && liveSourceFields(source, required)
}

func summarizeLiveFields(source map[string]any) map[string]any {
	fields := map[string]any{}
	for name, value := range source {
		switch value := value.(type) {
		case []any:
			fields[name] = map[string]any{"present": true, "count": len(value)}
		case map[string]any:
			fields[name] = map[string]any{"present": true, "keys": len(value)}
		default:
			fields[name] = map[string]any{"present": value != nil}
		}
	}
	return fields
}

// This opt-in check saves field names/counts only, never provider bodies or credentials.
func TestLiveProviderCompatibility(t *testing.T) {
	reportPath := os.Getenv("HOLYARRS_METADATA_LIVE_REPORT")
	tokenPath := os.Getenv("HOLYARRS_METADATA_TMDB_TOKEN_FILE")
	if reportPath == "" || tokenPath == "" {
		t.Skip("explicit report and private token-file paths required")
	}
	reportFile, tokenPath, err := openLiveReport(reportPath, tokenPath)
	if err != nil {
		t.Fatal("live report or credential path is unsafe")
	}
	records := []map[string]any{}
	passed := true
	defer func() {
		report := map[string]any{"passed": passed, "checked_at": time.Now().UTC(), "execution": "Windows direct adapter calls; no Oracle or public endpoint", "attempts_per_lookup": 1, "results": records, "permission_or_completeness_proved": false}
		if saveLiveReport(reportFile, report) != nil {
			t.Error("cannot save sanitized live report")
		}
		if !passed {
			t.Error("one or more live adapter contracts failed; see sanitized report")
		}
	}()
	client, err := New(Options{
		Providers: map[string]Config{
			"tmdb":        {KeyFile: tokenPath, Interval: 200 * time.Millisecond},
			"tvmaze":      {Interval: time.Second},
			"musicbrainz": {Interval: 1100 * time.Millisecond},
		},
		Timeout: 15 * time.Second, Attempts: 1,
		UserAgent: "HolyArrsMetadata/0.1 (https://github.com/CoderLuii/HolyArrs; private compatibility check)",
	})
	if err != nil {
		passed = false
		t.Fatal("cannot initialize live provider client")
	}
	fetch := func(provider, kind, operation, id string, season int, search string, required map[string]string) map[string]any {
		q := model.Query{Provider: provider, Kind: kind, Operation: operation, ID: id, Season: season, Q: search, Language: "en-US", Page: 1, Limit: 20}
		started := time.Now()
		result, err := client.Fetch(context.Background(), q)
		record := map[string]any{"provider": provider, "kind": kind, "operation": operation, "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000}
		records = append(records, record)
		if err != nil {
			passed = false
			record["passed"] = false
			var failure *model.FetchError
			if errors.As(err, &failure) {
				record["code"] = failure.Code
				record["status"] = failure.Status
			} else {
				record["code"] = "adapter_failure"
			}
			return nil
		}
		var data map[string]any
		if json.Unmarshal(result.Data, &data) != nil || !result.Complete {
			passed = false
			record["passed"] = false
			record["code"] = "incomplete_result"
			return nil
		}
		record["normalized_bytes"] = len(result.Data)
		if source, ok := data["source_data"].(map[string]any); ok {
			record["source_fields"] = summarizeLiveFields(source)
			if !liveDetailContract(data, provider, kind, operation, id, season, required) {
				passed = false
				record["passed"] = false
				record["code"] = "field_contract"
				return nil
			}
			record["passed"] = true
			record["field_contract"] = true
			return source
		}
		if items, ok := data["items"].([]any); ok {
			record["search_items"] = len(items)
			if len(items) != 0 {
				if item, ok := items[0].(map[string]any); ok {
					if source, ok := item["source_data"].(map[string]any); ok {
						record["first_item_source_fields"] = summarizeLiveFields(source)
					}
				}
			}
		}
		if !liveSearchContract(data, provider, kind, required) {
			passed = false
			record["passed"] = false
			record["code"] = "field_contract"
			return nil
		}
		record["passed"] = true
		record["field_contract"] = true
		return nil
	}
	fetch("tmdb", "movie", "detail", "550", 0, "", map[string]string{"id": "number", "title": "string", "release_date": "string"})
	fetch("tmdb", "show", "detail", "1396", 0, "", map[string]string{"id": "number", "name": "string", "number_of_seasons": "number", "seasons": "array"})
	fetch("tmdb", "show", "season", "1396", 1, "", map[string]string{"id": "number", "name": "string", "season_number": "number", "episodes": "array"})
	fetch("tvmaze", "show", "detail", "1", 0, "", map[string]string{"id": "number", "name": "string", "_embedded": "object"})
	fetch("tvmaze", "show", "search", "", 0, "The Office", map[string]string{"id": "number", "name": "string"})
	firstID := func(source map[string]any, field string) string {
		values, _ := source[field].([]any)
		if len(values) != 0 {
			item, _ := values[0].(map[string]any)
			id, _ := item["id"].(string)
			return id
		}
		return ""
	}
	artist := fetch("musicbrainz", "artist", "detail", "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d", 0, "", map[string]string{"id": "string", "name": "string", "release-groups": "array"})
	if artist == nil {
		return
	}
	groupID := firstID(artist, "release-groups")
	if groupID == "" {
		passed = false
		return
	}
	group := fetch("musicbrainz", "release-group", "detail", groupID, 0, "", map[string]string{"id": "string", "title": "string", "releases": "array"})
	if group == nil {
		return
	}
	releaseID := firstID(group, "releases")
	if releaseID == "" {
		passed = false
		return
	}
	release := fetch("musicbrainz", "release", "detail", releaseID, 0, "", map[string]string{"id": "string", "title": "string", "media": "array"})
	if release == nil {
		return
	}
	media, _ := release["media"].([]any)
	if len(media) != 0 {
		medium, _ := media[0].(map[string]any)
		tracks, _ := medium["tracks"].([]any)
		if len(tracks) != 0 {
			track, _ := tracks[0].(map[string]any)
			recording, _ := track["recording"].(map[string]any)
			if id, ok := recording["id"].(string); ok {
				fetch("musicbrainz", "recording", "detail", id, 0, "", map[string]string{"id": "string", "title": "string"})
				return
			}
		}
	}
	passed = false
}
