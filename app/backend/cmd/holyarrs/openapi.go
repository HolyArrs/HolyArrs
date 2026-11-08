package main

import (
	"bytes"
	_ "embed"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// openapiJSON describes every /api/v1 route. It is generated from the code
// (routes, permissions, scopes, parts) by TestOpenAPIGenerated, which also
// fails when the committed file and the code disagree.
//
//go:generate go test -run TestOpenAPIGenerated -update .
//go:embed openapi.json
var openapiJSON []byte

// openapi serves the file; under a URL base its server address is the base.
func (a *handlers) openapi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	b := openapiJSON
	if base := webserver.URLBaseFrom(r.Context()); base != "" {
		b = bytes.Replace(b, []byte(`"servers": [
    {
      "url": "/"`), []byte(`"servers": [
    {
      "url": "`+base+`/"`), 1)
	}
	_, _ = w.Write(b)
}
