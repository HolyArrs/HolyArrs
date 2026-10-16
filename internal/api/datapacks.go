package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/datapack"
)

// dataPackRoutes is wired into Handler by the shared route-registration patch.
func (a *API) dataPackRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/data-packs", a.listDataPacks)
	m.HandleFunc("GET /api/v1/data-packs/{id}", a.exportDataPack)
	m.HandleFunc("GET /api/v1/data-packs/{id}/preview", a.previewDataPack)
	m.HandleFunc("POST /api/v1/data-packs/validate", a.validateDataPack)
	m.HandleFunc("POST /api/v1/data-packs", a.importDataPack)
	m.HandleFunc("POST /api/v1/data-packs/{id}/apply", a.applyDataPack)
}

func (a *API) listDataPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := datapack.List(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	if packs == nil {
		packs = []datapack.Summary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data_packs": packs})
}

func (a *API) validateDataPack(w http.ResponseWriter, r *http.Request) {
	raw, ok := readDataPack(w, r)
	if !ok {
		return
	}
	p, err := datapack.Decode(raw)
	if errors.Is(err, datapack.ErrTooLarge) {
		fail(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "pack": dataPackSummary(p)})
}

func (a *API) importDataPack(w http.ResponseWriter, r *http.Request) {
	raw, ok := readDataPack(w, r)
	if !ok {
		return
	}
	p, err := datapack.Import(r.Context(), a.s.Store, raw, actor(r))
	switch {
	case errors.Is(err, datapack.ErrConflict):
		fail(w, http.StatusConflict, "a data pack with this id already exists")
	case errors.Is(err, datapack.ErrTooLarge):
		fail(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, datapack.ErrInvalidInput):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internal(w, err)
	default:
		writeJSON(w, http.StatusCreated, map[string]any{"pack": dataPackSummary(p)})
	}
}

func (a *API) exportDataPack(w http.ResponseWriter, r *http.Request) {
	b, err := datapack.Export(r.Context(), a.s.Store, r.PathValue("id"))
	switch {
	case errors.Is(err, datapack.ErrNotFound):
		fail(w, http.StatusNotFound, "data pack not found")
		return
	case errors.Is(err, datapack.ErrInvalidInput):
		fail(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(b, '\n'))
}

func (a *API) previewDataPack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for key := range q {
		if key != "pick_rule_id" && key != "naming_id" {
			fail(w, http.StatusBadRequest, "preview accepts only pick_rule_id and naming_id")
			return
		}
	}
	namingIDs := q["naming_id"]
	if len(namingIDs) > 1 {
		fail(w, http.StatusBadRequest, "select at most one naming_id")
		return
	}
	selection := datapack.Selection{PickRuleIDs: q["pick_rule_id"]}
	if len(namingIDs) == 1 {
		selection.NamingID = namingIDs[0]
	}
	preview, err := datapack.Preview(r.Context(), a.s.Store, r.PathValue("id"), selection)
	switch {
	case errors.Is(err, datapack.ErrNotFound):
		fail(w, http.StatusNotFound, "data pack not found")
	case errors.Is(err, datapack.ErrInvalidInput):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internal(w, err)
	default:
		writeJSON(w, http.StatusOK, preview)
	}
}

func (a *API) applyDataPack(w http.ResponseWriter, r *http.Request) {
	raw, ok := readRequestBody(w, r, 16<<10, "request is too large")
	if !ok {
		return
	}
	var body struct {
		PickRuleIDs  []string `json:"pick_rule_ids"`
		NamingID     string   `json:"naming_id"`
		ApprovalHash string   `json:"approval_hash"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&body); err != nil || strings.TrimSpace(body.ApprovalHash) == "" {
		fail(w, http.StatusBadRequest, "send pick_rule_ids, optional naming_id and approval_hash")
		return
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "send one JSON object")
		return
	}
	result, err := datapack.Apply(r.Context(), a.s.Store, r.PathValue("id"),
		datapack.Selection{PickRuleIDs: body.PickRuleIDs, NamingID: body.NamingID}, body.ApprovalHash, actor(r))
	switch {
	case errors.Is(err, datapack.ErrNotFound):
		fail(w, http.StatusNotFound, "data pack not found")
	case errors.Is(err, datapack.ErrConflict):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, datapack.ErrStaleApproval):
		fail(w, http.StatusConflict, "data pack preview is stale; preview the selection again")
	case errors.Is(err, datapack.ErrInvalidInput):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internal(w, err)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func readDataPack(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	return readRequestBody(w, r, datapack.MaxBytes, datapack.ErrTooLarge.Error())
}

func readRequestBody(w http.ResponseWriter, r *http.Request, limit int64, tooLarge string) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err == nil {
		return b, true
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		fail(w, http.StatusRequestEntityTooLarge, tooLarge)
		return nil, false
	}
	internal(w, err)
	return nil, false
}

func dataPackSummary(p datapack.Pack) datapack.Summary {
	return datapack.Summary{ID: p.ID, Name: p.Name, Status: "inactive", PickRules: len(p.PickRules), NamingSets: len(p.Naming), ReleaseTests: len(p.ReleaseTests)}
}
