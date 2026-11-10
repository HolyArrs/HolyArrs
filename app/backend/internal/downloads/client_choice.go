package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// clientTimeout bounds one call to a client. A busy NAS client (for example
// during a recheck) can take longer than 15 s to answer.
const clientTimeout = 30 * time.Second

// Back-off after a Downloader fails: 1 minute, doubling, at most 30.
const (
	backoffFirst = time.Minute
	backoffMost  = 30 * time.Minute
)

// Setting keys for the Downloader settings group.
const (
	SettingStallAfter      = "stalled_after_hours"
	SettingActAfter        = "stalled_act_hours"
	SettingSpacing         = "grab_spacing_seconds"
	SettingRemoveFailed    = "remove_failed"
	SettingFailDangerous   = "fail_dangerous"
	SettingReplaceVanished = "replace_vanished"
)

// settings holds the Downloader settings (group links), read once per sync.
type settings struct {
	stallAfter, actAfter time.Duration
	spacing              time.Duration
	removeFailed         bool
	failDangerous        bool
	replaceVanished      bool
}

// Client returns the HTTP client for one Downloader. It never follows a redirect
// to another host or scheme. Without allowLocal it refuses this machine and the
// local network, checking every address after DNS. Link-local and cloud metadata
// addresses (outbound.NotMetadata) and metadata names are refused even with
// allowLocal.
func Client(base string, allowLocal bool) *http.Client {
	allow := outbound.NotMetadata
	if !allowLocal {
		allow = outbound.PublicOnly
	}
	d := outbound.PolicyDialer(allow)
	host, scheme := "", ""
	if u, err := url.Parse(base); err == nil {
		host, scheme = u.Host, u.Scheme
	}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, _, err := net.SplitHostPort(addr); err == nil && metadataName(h) {
			return nil, &net.OpError{Op: "dial", Net: network, Err: outbound.ErrAddressRefused}
		}
		return d.DialContext(ctx, network, addr)
	}
	return &http.Client{
		Timeout:   clientTimeout,
		Transport: &http.Transport{Proxy: nil, DialContext: dial, TLSHandshakeTimeout: 10 * time.Second, MaxIdleConnsPerHost: 2},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Host != host || r.URL.Scheme != scheme || len(via) > 3 {
				return errors.New("the Downloader sent us to another address")
			}
			return nil
		},
	}
}

// Check asks one Downloader for its version and records the answer.
func (m *Manager) Check(ctx context.Context, d domain.Downloader) (Health, error) {
	e, err := m.engine(ctx, d)
	if err != nil {
		return Health{}, err
	}
	h, err := e.Health(ctx)
	if err == nil {
		raw := e
		if guarded, ok := e.(*guardedEngine); ok {
			raw = guarded.Engine
		}
		raw = unwrapReadOnly(raw)
		if s, ok := raw.(*Sabnzbd); ok {
			for _, c := range d.Categories() {
				if c == "" || c == "*" {
					continue
				}
				if has, cerr := s.HasCategory(ctx, c); cerr == nil && !has {
					err = fmt.Errorf("%w (%s)", ErrNoCategory, c)
					break
				}
			}
		}
	}
	if err != nil {
		if !errors.Is(err, ErrNoCategory) {
			m.downloaderFailed(ctx, d, err)
		}
		return h, err
	}
	m.downloaderOK(ctx, d)
	if m.Sync != nil && !m.Sync.View(d.ID).Fresh {
		m.Kick(d.ID) // The client answers again, so let the reader catch up now.
	}
	if d.CheckedAt == nil || m.now().Sub(*d.CheckedAt) > time.Minute {
		_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE downloaders SET checked_at = ? WHERE id = ?", database.FormatTime(m.now()), d.ID)
			return err
		})
	}
	return h, nil
}

// CheckAll asks every Downloader that is not paused or removed, once.
func (m *Manager) CheckAll(ctx context.Context) {
	dls, err := database.Select[domain.Downloader](ctx, m.Store.Read(), "downloaders", "removed = 0 AND state != 'disabled' ORDER BY id")
	if err != nil {
		return
	}
	for _, d := range dls {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, _ = m.Check(cctx, d)
		cancel()
	}
}

// metadataName reports whether h names a cloud metadata service.
func metadataName(h string) bool {
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	switch h {
	case "metadata", "metadata.google.internal", "metadata.goog", "instance-data", "instance-data.ec2.internal", "metadata.azure.com", "metadata.platformequinix.com":
		return true
	}
	return false
}

// order lists the Downloaders a Release may go to now: by priority, with equal
// priorities taking turns, leaving out paused and resting ones. It also gives
// the reason for each one skipped.
func (m *Manager) order(all []domain.Downloader, now time.Time) ([]domain.Downloader, []string) {
	var ok []domain.Downloader
	var skipped []string
	for _, d := range all {
		switch {
		case d.State == "disabled":
			skipped = append(skipped, d.Name+" is paused")
		case d.BackoffUntil != nil && d.BackoffUntil.After(now):
			skipped = append(skipped, fmt.Sprintf("%s is resting until %s after failing", d.Name, d.BackoffUntil.UTC().Format("15:04 UTC")))
		default:
			ok = append(ok, d)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rr == nil {
		m.rr = map[string]int{}
	}
	var out []domain.Downloader
	for i := 0; i < len(ok); {
		j := i
		for j < len(ok) && ok[j].Priority == ok[i].Priority {
			j++
		}
		group := ok[i:j]
		key := ok[i].Protocol + ":" + strconv.Itoa(ok[i].Priority)
		turn := m.rr[key] % len(group)
		out = append(out, group[turn:]...)
		out = append(out, group[:turn]...)
		if len(group) > 1 {
			m.rr[key]++
		}
		i = j
	}
	return out, skipped
}

// space waits until the last send to the same host is at least the grab spacing
// old, then holds that host until the returned func is called.
func (m *Manager) space(ctx context.Context, base string) (func(), error) {
	host := base
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		host = u.Hostname()
	}
	gap := m.settings(ctx).spacing
	m.mu.Lock()
	if m.hosts == nil {
		m.hosts = map[string]chan struct{}{}
		m.lastSend = map[string]time.Time{}
	}
	hm := m.hosts[host]
	if hm == nil {
		hm = make(chan struct{}, 1)
		m.hosts[host] = hm
	}
	m.mu.Unlock()
	select {
	case hm <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.mu.Lock()
	last, seen := m.lastSend[host]
	m.mu.Unlock()
	if wait := last.Add(gap).Sub(m.now()); seen && wait > 0 {
		if err := m.sleep(ctx, wait); err != nil {
			<-hm
			return nil, err
		}
	}
	return func() {
		m.mu.Lock()
		m.lastSend[host] = m.now()
		m.mu.Unlock()
		<-hm
	}, nil
}

func (m *Manager) sleep(ctx context.Context, d time.Duration) error {
	if m.Sleep != nil {
		return m.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// downloaderFailed records a failed call: the state in plain words, the error
// (never a key, since adapters strip addresses) and a back-off.
func (m *Manager) downloaderFailed(ctx context.Context, d domain.Downloader, cause error) {
	state := "unreachable"
	if errors.Is(cause, ErrAuth) {
		state = "auth_failed"
	}
	log.Printf("downloaders: %s (%d): %s", d.Name, d.ID, firstLine(cause.Error()))
	now := m.now()
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		var fails int
		var cur string
		if err := tx.QueryRow("SELECT fail_count, state FROM downloaders WHERE id = ?", d.ID).Scan(&fails, &cur); err != nil {
			return err
		}
		if cur == "disabled" {
			state = cur // A pause set by a person stays.
		}
		wait := backoffFirst << min(fails, 5)
		if wait > backoffMost {
			wait = backoffMost
		}
		_, err := tx.Exec("UPDATE downloaders SET state = ?, fail_count = fail_count + 1, backoff_until = ?, last_error = ?, checked_at = ?, updated_at = ? WHERE id = ?",
			state, database.FormatTime(now.Add(wait)), firstLine(cause.Error()), database.FormatTime(now), database.FormatTime(now), d.ID)
		return err
	})
	m.publish("downloader", map[string]any{"id": d.ID, "state": state})
}

// downloaderOK ends a back-off. It writes only when something changes.
func (m *Manager) downloaderOK(ctx context.Context, d domain.Downloader) {
	if (d.State == "ok" || d.State == "disabled") && d.FailCount == 0 && d.BackoffUntil == nil && d.LastError == "" {
		return
	}
	now := m.now()
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloaders SET state = CASE WHEN state = 'disabled' THEN state ELSE 'ok' END, fail_count = 0, backoff_until = NULL,
			last_error = '', checked_at = ?, updated_at = ? WHERE id = ?`, database.FormatTime(now), database.FormatTime(now), d.ID)
		return err
	})
	m.publish("downloader", map[string]any{"id": d.ID, "state": "ok"})
}

func (m *Manager) settings(ctx context.Context) settings {
	st := settings{stallAfter: 6 * time.Hour, actAfter: 24 * time.Hour, spacing: 2 * time.Second, removeFailed: true, failDangerous: true, replaceVanished: true}
	rows, err := m.Store.Read().QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = 'links'")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) != nil {
				continue
			}
			n, nerr := strconv.ParseFloat(v, 64)
			switch k {
			case SettingStallAfter:
				if nerr == nil && n > 0 {
					st.stallAfter = time.Duration(n * float64(time.Hour))
				}
			case SettingActAfter:
				if nerr == nil && n > 0 {
					st.actAfter = time.Duration(n * float64(time.Hour))
				}
			case SettingSpacing:
				if nerr == nil && n >= 0 {
					st.spacing = time.Duration(n * float64(time.Second))
				}
			case SettingRemoveFailed:
				st.removeFailed = v != "false"
			case SettingFailDangerous:
				st.failDangerous = v != "false"
			case SettingReplaceVanished:
				st.replaceVanished = v != "false"
			}
		}
	}
	if m.StallAfter > 0 {
		st.stallAfter = m.StallAfter
	}
	if m.ActAfter > 0 {
		st.actAfter = m.ActAfter
	}
	return st
}
