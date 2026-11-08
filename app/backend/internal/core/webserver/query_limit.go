package webserver

import (
	"net/http"
	"strconv"
)

// QueryLimit reads ?limit= (1-500); anything else gives def.
func QueryLimit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}
