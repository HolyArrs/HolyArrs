package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/applog"
	"github.com/HolyArrs/holyarrs/app/backend/internal/doctor"
)

// Extra masks for the support bundle, on top of the log's own masking:
// whole Authorization and API key headers, and key-like values after = or :.
var (
	bundleHeader = regexp.MustCompile(`(?i)\b((?:proxy-)?authorization|x-api-key|x-plex-token|cookie|set-cookie)("?\s*[:=]\s*)[^\r\n]*`)
	bundlePair   = regexp.MustCompile(`(?i)\b(api[-_ ]?key|apikey|token|access[-_ ]?token|refresh[-_ ]?token|password|passwd|passkey|secret)("?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s"',;&]+)`)
)

// redactBundle masks secrets in text once more, line by line, before it
// goes into the support bundle.
func redactBundle(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = applog.RedactPatterns(l)
		l = bundleHeader.ReplaceAllString(l, "${1}${2}"+applog.Hidden)
		l = bundlePair.ReplaceAllStringFunc(l, func(m string) string {
			sm := bundlePair.FindStringSubmatch(m)
			if strings.Trim(sm[3], `"'`) == applog.Hidden {
				return m
			}
			return sm[1] + sm[2] + applog.Hidden
		})
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// supportBundle is a zip to attach to a bug report: the newest log (masked,
// then masked again), the About view, the doctor report and the health data.
// Owner and admins only, like the log download.
func (a *API) supportBundle(w http.ResponseWriter, r *http.Request) {
	logText := "this HolyArrs keeps no log file; read the program's output (docker logs)\n"
	if a.s.Log != nil {
		if a.s.Secrets != nil {
			if err := a.s.Log.Sync(r.Context(), a.s.Secrets); err != nil {
				internal(w, err)
				return
			}
		}
		logText = string(a.s.Log.Tail(maxLogDownload))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rep := doctor.Run(ctx, doctor.Options{Version: a.s.Version, ConfigDir: a.configDir(), DB: a.s.Store.Read(), SavePath: a.s.SavePath,
		FolderBudget: 10 * time.Second, NetTimeout: 5 * time.Second})
	about, _ := json.MarshalIndent(a.aboutView(r.Context()), "", "  ")
	health, _ := json.MarshalIndent(a.healthView(r.Context()), "", "  ")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now().UTC()
	for _, f := range []struct{ name, body string }{
		{"log.txt", logText},
		{"about.json", string(about) + "\n"},
		{"doctor.txt", rep.Text()},
		{"health.json", string(health) + "\n"},
	} {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: now})
		if err != nil {
			internal(w, err)
			return
		}
		if _, err := fw.Write([]byte(redactBundle(f.body))); err != nil {
			internal(w, err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="holyarrs-support-`+now.Format("20060102-150405")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}
