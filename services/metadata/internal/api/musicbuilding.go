package api

import (
	"encoding/json"
	"net/http"
)

// chartsBuilding answers "Building" while the chart gate is closed (music
// data plan S5, AC 10 and 24): charts, genre pages, new releases, the
// trending lists and the shelves stay hidden until every chart artist
// and its similar artists have a photo and every album a cover or a
// recorded miss. True when it answered. The answer is a 200 the edge
// keeps a minute only (partialHeader), so the filled charts show soon
// after the gate opens.
func (s *Server) chartsBuilding(w http.ResponseWriter, r *http.Request) bool {
	if s.opts.ChartsReady == nil || s.opts.ChartsReady() {
		return false
	}
	b, _ := json.Marshal(map[string]any{"schema_version": 2, "status": "building", "building": true, "items": []any{}, "genres": []any{}})
	h := w.Header()
	h.Set(partialHeader, "building")
	h.Set("Cache-Control", "private, max-age=60")
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(b)
	return true
}
