package arrapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The format routes take what Profilarr pushes: formats kept as sent and
// listed back unchanged, each turned into a trait through the same reader
// an old-app import uses. A format testing something HolyArrs cannot is
// kept but becomes no trait; skipped says why. Only formats sent here are
// listed or removed, never traits made in HolyArrs.

// wireFormat is a format as Profilarr sends it.
type wireFormat struct {
	Name           string     `json:"name"`
	Specifications []wireSpec `json:"specifications"`
}

// wireSpec is one test of a format. Fields come as a list of name and
// value pairs (an object of them is read too).
type wireSpec struct {
	Name           string          `json:"name"`
	Implementation string          `json:"implementation"`
	Negate         bool            `json:"negate"`
	Required       bool            `json:"required"`
	Fields         json.RawMessage `json:"fields"`
}

// mayWriteRules reports whether the caller may change formats and
// profiles: they change what HolyArrs picks, so the key must act as someone
// who may change settings. It returns false after sending the answer.
func mayWriteRules(w http.ResponseWriter, r *http.Request) bool {
	if who, ok := webserver.From(r.Context()); !ok || !who.Has(webserver.PermSettings) {
		message(w, http.StatusForbidden, "the key acts as the person who made it, who may not change settings")
		return false
	}
	return true
}

// readRaw reads a JSON body into v and also answers it as sent. It
// returns false after sending a 400.
func readRaw(w http.ResponseWriter, r *http.Request, v any) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyMax))
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil || bytes.TrimSpace(b)[0] != '{' {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return nil, false
	}
	return b, true
}

// asMap reads a stored body with its numbers as sent.
func asMap(body string) map[string]any {
	d := json.NewDecoder(strings.NewReader(body))
	d.UseNumber()
	m := map[string]any{}
	_ = d.Decode(&m)
	return m
}

// fieldValues reads a test's fields by lower-case name.
func fieldValues(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	var list []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
	if json.Unmarshal(raw, &list) == nil {
		for _, f := range list {
			out[strings.ToLower(f.Name)] = f.Value
		}
		return out
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			out[strings.ToLower(k)] = v
		}
	}
	return out
}

func fieldText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func fieldNum(v any) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(fieldText(v)), 64)
	return n
}

// formatSpecs reads a format's tests as the converter takes them.
func (h *handler) formatSpecs(f wireFormat) []library.FormatSpec {
	out := make([]library.FormatSpec, 0, len(f.Specifications))
	for _, s := range f.Specifications {
		v := fieldValues(s.Fields)
		fs := library.FormatSpec{Name: s.Name, Implementation: s.Implementation, Negate: s.Negate, Required: s.Required,
			Value: fieldText(v["value"]), Min: fieldNum(v["min"]), Max: fieldNum(v["max"]), Except: v["exceptlanguage"] == true}
		if s.Implementation == "LanguageSpecification" {
			fs.Language = languageCode(sideLanguages[h.side][int64(fieldNum(v["value"]))])
		}
		out = append(out, fs)
	}
	return out
}

// formatShape is a stored format as the list answers it: as sent, with
// its id, and a missing rename switch or test list filled in.
func formatShape(f pushedFormat) map[string]any {
	m := asMap(f.Body)
	m["id"] = f.ID
	m["name"] = f.Name
	if _, ok := m["includeCustomFormatWhenRenaming"]; !ok {
		m["includeCustomFormatWhenRenaming"] = false
	}
	if m["specifications"] == nil {
		m["specifications"] = []any{}
	}
	return m
}

func (h *handler) listFormats(w http.ResponseWriter, r *http.Request) {
	fs, err := listFormatRows(r.Context(), h.Store.Read(), h.side)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, formatShape(f))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) getFormat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	f, err := formatRow(r.Context(), h.Store.Read(), h.side, id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, formatShape(f))
}

func (h *handler) addFormat(w http.ResponseWriter, r *http.Request) {
	h.saveFormat(w, r, 0)
}

func (h *handler) putFormat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.saveFormat(w, r, id)
}

// saveFormat stores a new format (id 0) or replaces one, then makes,
// changes or removes the trait it becomes.
func (h *handler) saveFormat(w http.ResponseWriter, r *http.Request, id int64) {
	if !mayWriteRules(w, r) {
		return
	}
	ctx := r.Context()
	var in wireFormat
	raw, ok := readRaw(w, r, &in)
	if !ok {
		return
	}
	f := pushedFormat{ID: id, Side: h.side, Name: in.Name, Body: string(raw)}
	if id != 0 {
		old, err := formatRow(ctx, h.Store.Read(), h.side, id)
		if err != nil {
			internalError(w, err)
			return
		}
		f.TraitID = old.TraitID
	}
	if strings.TrimSpace(f.Name) == "" {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "Name must not be empty", AttemptedValue: in.Name})
		return
	}
	if taken, err := formatNameTaken(ctx, h.Store.Read(), h.side, f.Name, id); err != nil || taken {
		if err != nil {
			internalError(w, err)
			return
		}
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "Must be unique", AttemptedValue: f.Name})
		return
	}
	specs, kinds := library.ConvertFormat(h.side, h.formatSpecs(in), library.ConvertOptions{MoreKinds: true})
	if len(kinds) > 0 {
		f.Skipped = "it tests " + strings.Join(kinds, ", ") + ", which HolyArrs cannot test"
		log.Printf("arrapi: %s format %q becomes no trait: %s", h.side, clipName(f.Name), f.Skipped)
	}
	by := actor(r)
	if f.Skipped != "" && f.TraitID != 0 {
		if _, err := library.DeleteOwnTrait(ctx, h.Store, f.TraitID, by); err != nil && !errors.Is(err, database.ErrNotFound) {
			internalError(w, err)
			return
		}
		f.TraitID = 0
	}
	newID, err := saveFormatRow(ctx, h.Store, f, by)
	if err != nil {
		internalError(w, err)
		return
	}
	f.ID = newID
	if f.Skipped == "" {
		trait, err := library.SavePushedTrait(ctx, h.Store, h.side, h.side+":"+itoa(f.ID), f.Name, specs, by)
		if err == nil {
			err = setFormatTrait(ctx, h.Store, h.side, f.ID, trait)
		}
		if err != nil {
			if id == 0 {
				_ = deleteFormatRow(ctx, h.Store, h.side, f.ID)
			}
			internalError(w, err)
			return
		}
		f.TraitID = trait
	}
	code := http.StatusAccepted
	if id == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, formatShape(f))
}

// deleteFormat removes a format sent here and its trait, which leaves every
// Pick rules it scored.
func (h *handler) deleteFormat(w http.ResponseWriter, r *http.Request) {
	if !mayWriteRules(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	f, err := formatRow(ctx, h.Store.Read(), h.side, id)
	if err != nil {
		internalError(w, err)
		return
	}
	if f.TraitID != 0 {
		if _, err := library.DeleteOwnTrait(ctx, h.Store, f.TraitID, actor(r)); err != nil && !errors.Is(err, database.ErrNotFound) {
			internalError(w, err)
			return
		}
	}
	if err := deleteFormatRow(ctx, h.Store, h.side, id); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// clipName keeps a sent name short in the log.
func clipName(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60])
	}
	return s
}
