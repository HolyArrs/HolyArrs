package webserver

import (
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
)

// ServeAttachment sends the file at path under the file name name, with
// range requests, HEAD and If-Modified-Since. A Content-Type the caller set
// is kept; without one it comes from the name's extension or the bytes. With download set the browser
// is told to save it as name; a name outside ASCII goes as filename* (RFC
// 2231). A missing file, or a folder, answers 404. Errors never carry the
// path, which may hold a title.
func ServeAttachment(w http.ResponseWriter, r *http.Request, path, name string, download bool) {
	f, err := os.Open(path)
	if err != nil {
		attachmentFailed(w, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		attachmentFailed(w, err)
		return
	}
	if st.IsDir() {
		Fail(w, http.StatusNotFound, "not found")
		return
	}
	if download {
		cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
		if cd == "" {
			cd = "attachment"
		}
		w.Header().Set("Content-Disposition", cd)
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func attachmentFailed(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		Fail(w, http.StatusNotFound, "not found")
		return
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	log.Printf("webserver: a file could not be sent: %v", err)
	Fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}
