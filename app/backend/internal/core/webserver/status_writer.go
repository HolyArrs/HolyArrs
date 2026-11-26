package webserver

import "net/http"

// StatusWriter remembers the status a handler answered with.
type StatusWriter struct {
	http.ResponseWriter
	Code int
}

func (s *StatusWriter) WriteHeader(code int) {
	if s.Code == 0 {
		s.Code = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *StatusWriter) Write(b []byte) (int, error) {
	if s.Code == 0 {
		s.Code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap gives http.ResponseController the underlying writer, so a handler
// behind it can still move its read deadline (the restore upload) or flush.
func (s *StatusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
