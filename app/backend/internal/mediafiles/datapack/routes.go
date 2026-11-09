package datapack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Deps is what the data pack routes need.
type Deps struct {
	Store *database.Store
}

// Handlers serves the data pack routes under /api/v1/data-packs.
type Handlers struct {
	Deps
}

// Routes registers the data pack routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/data-packs", h.listDataPacks)
	m.HandleFunc("GET /api/v1/data-packs/{id}", h.exportDataPack)
	m.HandleFunc("GET /api/v1/data-packs/{id}/preview", h.previewDataPack)
	m.HandleFunc("POST /api/v1/data-packs/validate", h.validateDataPack)
	m.HandleFunc("POST /api/v1/data-packs", h.importDataPack)
	m.HandleFunc("POST /api/v1/data-packs/{id}/apply", h.applyDataPack)
}

func (h *Handlers) listDataPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := List(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if packs == nil {
		packs = []Summary{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"data_packs": packs})
}

func (h *Handlers) validateDataPack(w http.ResponseWriter, r *http.Request) {
	raw, ok := readDataPack(w, r)
	if !ok {
		return
	}
	p, err := Decode(raw)
	if errors.Is(err, ErrTooLarge) {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "pack": dataPackSummary(p)})
}

func (h *Handlers) importDataPack(w http.ResponseWriter, r *http.Request) {
	raw, ok := readDataPack(w, r)
	if !ok {
		return
	}
	p, err := Import(r.Context(), h.Store, raw, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	switch {
	case errors.Is(err, ErrConflict):
		webserver.Fail(w, http.StatusConflict, "a data pack with this id already exists")
	case errors.Is(err, ErrTooLarge):
		webserver.Fail(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, ErrInvalidInput):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		webserver.WriteJSON(w, http.StatusCreated, map[string]any{"pack": dataPackSummary(p)})
	}
}

func (h *Handlers) exportDataPack(w http.ResponseWriter, r *http.Request) {
	b, err := Export(r.Context(), h.Store, r.PathValue("id"))
	switch {
	case errors.Is(err, ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "data pack not found")
		return
	case errors.Is(err, ErrInvalidInput):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		webserver.Internal(w, "api", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(b, '\n'))
}

func (h *Handlers) previewDataPack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for key := range q {
		if key != "pick_rule_id" && key != "naming_id" {
			webserver.Fail(w, http.StatusBadRequest, "preview accepts only pick_rule_id and naming_id")
			return
		}
	}
	namingIDs := q["naming_id"]
	if len(namingIDs) > 1 {
		webserver.Fail(w, http.StatusBadRequest, "select at most one naming_id")
		return
	}
	selection := Selection{PickRuleIDs: q["pick_rule_id"]}
	if len(namingIDs) == 1 {
		selection.NamingID = namingIDs[0]
	}
	preview, err := Preview(r.Context(), h.Store, r.PathValue("id"), selection)
	switch {
	case errors.Is(err, ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "data pack not found")
	case errors.Is(err, ErrInvalidInput):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		webserver.WriteJSON(w, http.StatusOK, preview)
	}
}

func (h *Handlers) applyDataPack(w http.ResponseWriter, r *http.Request) {
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
		webserver.Fail(w, http.StatusBadRequest, "send pick_rule_ids, optional naming_id and approval_hash")
		return
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		webserver.Fail(w, http.StatusBadRequest, "send one JSON object")
		return
	}
	result, err := Apply(r.Context(), h.Store, r.PathValue("id"),
		Selection{PickRuleIDs: body.PickRuleIDs, NamingID: body.NamingID}, body.ApprovalHash, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	switch {
	case errors.Is(err, ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "data pack not found")
	case errors.Is(err, ErrConflict):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrStaleApproval):
		webserver.Fail(w, http.StatusConflict, "data pack preview is stale; preview the selection again")
	case errors.Is(err, ErrInvalidInput):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		webserver.WriteJSON(w, http.StatusOK, result)
	}
}

func readDataPack(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	return readRequestBody(w, r, MaxBytes, ErrTooLarge.Error())
}

func readRequestBody(w http.ResponseWriter, r *http.Request, limit int64, tooLarge string) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err == nil {
		return b, true
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, tooLarge)
		return nil, false
	}
	webserver.Internal(w, "api", err)
	return nil, false
}

func dataPackSummary(p Pack) Summary {
	return Summary{ID: p.ID, Name: p.Name, Status: "inactive", PickRules: len(p.PickRules), NamingSets: len(p.Naming), ReleaseTests: len(p.ReleaseTests)}
}
