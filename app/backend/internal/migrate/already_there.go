package migrate

import (
	"database/sql"
	"net/url"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// samePlace reports whether two addresses reach the same place: the same
// host and port (the scheme's usual port when none is written) and the same
// path, ignoring case and outer slashes.
func samePlace(a, b string) bool {
	ka, oka := addressKey(a)
	kb, okb := addressKey(b)
	return oka && okb && ka == kb
}

func addressKey(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		}
	}
	return strings.ToLower(u.Hostname()) + ":" + port + "/" + strings.ToLower(strings.Trim(u.Path, "/")), true
}

// sameSource names a Source here in the same format at the same address, or "".
func (r *run) sameSource(s *SourcePlan) string {
	have, err := database.Select[domain.Source](r.ctx, r.m.Store.Read(), "sources", "format = ? AND name NOT LIKE '% (removed %)' ORDER BY id", s.Format)
	if err != nil {
		return ""
	}
	for _, h := range have {
		if samePlace(h.URL, s.URL) {
			return h.Name
		}
	}
	return ""
}

// sameAlert names an Alert here of the same kind that writes to the same
// place (a Telegram chat, or the same mail server, sender and recipients), or "".
// ntfy and Gotify never match on the server alone: the topic or app token
// is not imported, so two Alerts on one server may go to different places.
func (r *run) sameAlert(a *AlertPlan) string {
	have, err := database.Select[domain.Alert](r.ctx, r.m.Store.Read(), "alerts", "kind = ? ORDER BY id", a.Kind)
	if err != nil {
		return ""
	}
	for _, h := range have {
		switch {
		case a.ChatID != "" && h.ChatID == a.ChatID:
			return h.Name
		case a.SMTPHost != "" && strings.EqualFold(h.SMTPHost, a.SMTPHost) && strings.EqualFold(h.SMTPFrom, a.SMTPFrom) && strings.EqualFold(strings.TrimSpace(h.StaffTo), strings.TrimSpace(a.StaffTo)):
			return h.Name
		}
	}
	return ""
}

// fillDownloader fills only the empty user name of the Downloader already
// here; everything the owner set stays, folder maps included (the report
// line names any to add by hand). It reports whether it filled anything.
func (r *run) fillDownloader(h domain.Downloader, d *DownloaderPlan) (bool, error) {
	if h.Username != "" || d.Username == "" {
		return false, nil
	}
	if r.dry {
		return true, nil
	}
	h.Username = clip(d.Username, 100)
	return true, r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		return database.Update(tx, "downloaders", &h, r.by)
	})
}
