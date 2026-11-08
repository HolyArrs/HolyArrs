package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Settings are the health check settings on This app.
type Settings struct {
	// EveryMinutes is how often the checks run (1 to 1440, default 5).
	EveryMinutes int `json:"every_minutes"`
	// FlapGuard is how many runs in a row a new result must hold before
	// an Alert is sent (1 to 20, default 2).
	FlapGuard int `json:"flap_guard"`
	// LowSpaceMB warns when a Library folder's disk has less free (default 1024).
	LowSpaceMB int `json:"low_space_mb"`
	// UpdateCheckURL is the release feed asked once a day (GitHub's
	// releases answer, JSON with tag_name or version, or plain text);
	// empty uses UpdateCheckDefault.
	UpdateCheckURL string `json:"update_check_url"`
	// UpdateCheck turns the update check on or off (off: no request at
	// all); UpdateCheckLocal lets the feed be on the local network. Nil in
	// a change keeps the current value.
	UpdateCheck      *bool `json:"update_check,omitempty"`
	UpdateCheckLocal *bool `json:"update_check_local,omitempty"`
	// UpdatePrerelease lets a pre-release (v1.2.0-rc1) give a notice;
	// default off. Nil in a change keeps the current value.
	UpdatePrerelease *bool `json:"update_prerelease,omitempty"`
	// UpdateCheckEveryDays is the days between update checks (1, 3, 7, 14
	// or 30; default 1). Nil in a change keeps the current value.
	UpdateCheckEveryDays *int `json:"update_check_every_days,omitempty"`
	// UpdateCheckDefault is the feed used when UpdateCheckURL is empty
	// (answered only).
	UpdateCheckDefault string `json:"update_check_default"`
}

// Update check settings in the this_app group, shown with the health
// settings; the updates module reads them.
const (
	settingUpdateURL   = "update_check_url"
	settingUpdateOn    = "update_check"
	settingUpdateLocal = "update_check_local"
	settingUpdatePre   = "update_prerelease"
	settingUpdateDays  = "update_check_every_days"
)

// updateDays are the allowed values of update_check_every_days.
var updateDays = []int{1, 3, 7, 14, 30}

// ErrSettings refuses settings out of range.
var ErrSettings = errors.New("every_minutes is 1 to 1440, flap_guard 1 to 20, low_space_mb 0 to 16777216, and update_check_url an http or https link without a name, password, query or fragment (it may hold no secret), or empty; update_check_every_days is 1, 3, 7, 14 or 30")

// ReadSettings reads the health check settings.
func ReadSettings(ctx context.Context, s *database.Store, defaultFeed string) (Settings, error) {
	e := &Engine{Store: s}
	out := Settings{FlapGuard: e.intSetting(ctx, settingGuard, 2, 1, 20), LowSpaceMB: e.intSetting(ctx, settingLowMB, 1024, 0, 1<<24),
		UpdateCheckURL: e.setting(ctx, settingUpdateURL), EveryMinutes: 5, UpdateCheckDefault: defaultFeed}
	on, local := e.setting(ctx, settingUpdateOn) != "off", e.setting(ctx, settingUpdateLocal) == "true"
	pre := e.setting(ctx, settingUpdatePre) == "true"
	days := e.intSetting(ctx, settingUpdateDays, 1, 1, 30)
	if !slices.Contains(updateDays, days) {
		days = 1
	}
	out.UpdateCheck, out.UpdateCheckLocal, out.UpdatePrerelease, out.UpdateCheckEveryDays = &on, &local, &pre, &days
	var every int
	err := s.Read().QueryRowContext(ctx, "SELECT every_seconds FROM schedules WHERE kind = ?", KindCheck).Scan(&every)
	if err == nil && every > 0 {
		out.EveryMinutes = every / 60
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	return out, nil
}

// SaveSettings checks and stores the health check settings.
func SaveSettings(ctx context.Context, s *database.Store, in Settings, by domain.Actor) error {
	in.UpdateCheckURL = strings.TrimSpace(in.UpdateCheckURL)
	if in.EveryMinutes < 1 || in.EveryMinutes > 1440 || in.FlapGuard < 1 || in.FlapGuard > 20 || in.LowSpaceMB < 0 || in.LowSpaceMB > 1<<24 {
		return ErrSettings
	}
	if in.UpdateCheckEveryDays != nil && !slices.Contains(updateDays, *in.UpdateCheckEveryDays) {
		return ErrSettings
	}
	if in.UpdateCheckURL != "" {
		u, err := url.Parse(in.UpdateCheckURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(in.UpdateCheckURL, "#") {
			return ErrSettings
		}
	}
	now := database.FormatTime(time.Now())
	byText, _ := json.Marshal(by)
	return s.Write(ctx, func(tx *sql.Tx) error {
		vals := map[string]string{settingGuard: strconv.Itoa(in.FlapGuard), settingLowMB: strconv.Itoa(in.LowSpaceMB), settingUpdateURL: in.UpdateCheckURL}
		if in.UpdateCheck != nil {
			vals[settingUpdateOn] = map[bool]string{true: "on", false: "off"}[*in.UpdateCheck]
		}
		if in.UpdatePrerelease != nil {
			vals[settingUpdatePre] = strconv.FormatBool(*in.UpdatePrerelease)
		}
		if in.UpdateCheckEveryDays != nil {
			vals[settingUpdateDays] = strconv.Itoa(*in.UpdateCheckEveryDays)
		}
		if in.UpdateCheckLocal != nil {
			vals[settingUpdateLocal] = strconv.FormatBool(*in.UpdateCheckLocal)
		}
		for k, v := range vals {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, string(byText), now, string(byText)); err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE schedules SET every_seconds = ?, updated_at = ?, updated_by = ? WHERE kind = ?", in.EveryMinutes*60, now, string(byText), KindCheck)
		return err
	})
}
