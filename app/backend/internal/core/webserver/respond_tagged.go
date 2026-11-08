package webserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// WriteTaggedJSON writes v with an ETag (a hash of the body) and, unless
// the handler set its own, "private, no-cache", so the browser keeps the
// answer and revalidates it; a matching If-None-Match gets 304.
func WriteTaggedJSON(w http.ResponseWriter, r *http.Request, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		Internal(w, "webserver", err)
		return
	}
	sum := sha256.Sum256(buf.Bytes())
	tag := `"` + hex.EncodeToString(sum[:12]) + `"`
	h := w.Header()
	h.Set("ETag", tag)
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-cache")
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		for _, t := range strings.Split(inm, ",") {
			if t = strings.TrimSpace(t); t == tag || t == "*" || strings.TrimPrefix(t, "W/") == tag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
