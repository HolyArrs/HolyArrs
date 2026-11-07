package webserver

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// maxJSONBody is the biggest JSON body Decode reads.
const maxJSONBody = 1 << 16

// WriteJSON answers with v as JSON under status code.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteJSONNoStore is WriteJSON for answers a browser or proxy must not keep,
// such as sign-in and session replies.
func WriteJSONNoStore(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, code, v)
}

// Fail answers with {"error": msg} under status code.
func Fail(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}

// FailNoStore is Fail with the no-store header.
func FailNoStore(w http.ResponseWriter, code int, msg string) {
	WriteJSONNoStore(w, code, map[string]string{"error": msg})
}

// Internal logs err under who and answers 500 without leaking it.
func Internal(w http.ResponseWriter, who string, err error) {
	log.Printf("%s: %v", who, err)
	Fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}

// Decode reads a JSON body into v. False means the answer (400 with help, or
// 413 for a body over the limit) is already sent.
func Decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(v)
	if tooBig(err) {
		Fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	if err != nil {
		Fail(w, http.StatusBadRequest, help)
		return false
	}
	return true
}

// DecodeKeys is Decode that also answers which top-level keys the body holds,
// so a left-out field can take a saved default.
func DecodeKeys(w http.ResponseWriter, r *http.Request, v any, help string) (map[string]json.RawMessage, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if tooBig(err) {
		Fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, false
	}
	keys := map[string]json.RawMessage{}
	if err != nil || json.Unmarshal(raw, v) != nil || json.Unmarshal(raw, &keys) != nil {
		Fail(w, http.StatusBadRequest, help)
		return nil, false
	}
	return keys, true
}

func tooBig(err error) bool {
	var mb *http.MaxBytesError
	return errors.As(err, &mb)
}
