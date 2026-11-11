package downloads

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

var (
	ErrName        = errors.New("give the Downloader a name of 1 to 80 characters")
	ErrNameUsed    = errors.New("another Downloader has that name")
	ErrAdapter     = errors.New("choose a supported Downloader kind")
	ErrURL         = errors.New("the address must start with http:// or https:// and name a host, with no name or password in it")
	ErrCategory    = errors.New("the category must be 1 to 64 characters with no slashes")
	ErrDelugeLabel = errors.New("Deluge labels use only lowercase letters, digits, _ - and .")
	ErrPriority    = errors.New("priority must be 1 to 50 (1 is used first)")
	ErrPathMap     = errors.New("each path map needs both paths, each starting with / or a drive letter")
	ErrUsername    = errors.New("the sign-in name must be at most 100 characters")
	ErrKeyNeed     = errors.New("SABnzbd needs its API key (SABnzbd: Config > General)")
	ErrQbitKey     = errors.New("qBittorrent needs an API key (qBittorrent 5.2 or newer: Options > Web UI > API key) or a user name and password")
	ErrKeyAgain    = errors.New("the address changed to another host; type the key or password again so it is never sent to a new host by itself")
	ErrNoVault     = errors.New("keys cannot be stored: the secrets vault is not open")
	ErrBusy        = errors.New("this Downloader still has active downloads; pause it and let them finish first (its category and address cannot change, and it cannot be removed, while they run)")
)

// Input holds the fields a caller sets when saving a Downloader.
type Input struct {
	Name        string  `json:"name"`
	Adapter     string  `json:"kind"`
	URL         string  `json:"url"`
	Username    *string `json:"username"`
	Secret      string  `json:"secret"`
	ClearSecret bool    `json:"clear_secret"`
	Category    *string `json:"category"`
	// Category for one kind of title; "" uses Category.
	CategoryMovies *string          `json:"category_movies"`
	CategoryShows  *string          `json:"category_shows"`
	CategoryMusic  *string          `json:"category_music"`
	Priority       *int             `json:"priority"`
	PathMap        []domain.PathMap `json:"path_map"`
	AllowLocal     *bool            `json:"allow_local_network"`
	Writable       *bool            `json:"writable"`
	ClearFinished  *bool            `json:"clear_finished"`
	Tags           []string         `json:"tags"`
}

// HealthView is the stored health of a Downloader as shown to users.
type HealthView struct {
	State string `json:"state"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

// Add saves a new Downloader and runs then in the same transaction.
func Add(ctx context.Context, s *database.Store, v *secrets.Vault, in Input, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (domain.Downloader, error) {
	var d domain.Downloader
	if err := fill(in, &d, true); err != nil {
		return d, err
	}
	if d.Adapter == "sabnzbd" && in.Secret == "" {
		return d, ErrKeyNeed
	}
	if d.Adapter == "qbittorrent" && in.Secret == "" {
		return d, ErrQbitKey
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, d.Name, 0); err != nil {
			return err
		}
		if err := putSecret(tx, v, &d, in.Secret, by); err != nil {
			return err
		}
		id, err := database.Insert(tx, "downloaders", &d, by)
		if err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return d, err
}

// Change updates the Downloader with the given id.
func Change(ctx context.Context, s *database.Store, v *secrets.Vault, id int64, in Input, by domain.Actor) (domain.Downloader, error) {
	var d domain.Downloader
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "downloaders", id, &d); err != nil {
			return err
		}
		if d.Removed {
			return database.ErrNotFound
		}
		if n, err := pendingAttemptCount(tx, id); err != nil {
			return err
		} else if n > 0 {
			return ErrPendingAttempt
		}
		oldHost, oldCat := hostOf(d.URL), strings.Join(d.Categories(), "/")
		oldLogin := d.Username != ""
		if err := fill(in, &d, false); err != nil {
			return err
		}
		if hostOf(d.URL) != oldHost || strings.Join(d.Categories(), "/") != oldCat {
			n, err := activeCount(tx, id)
			if err != nil {
				return err
			}
			if n > 0 {
				return ErrBusy
			}
		}
		if err := nameUsed(tx, d.Name, id); err != nil {
			return err
		}
		if in.ClearSecret {
			if d.Adapter == "sabnzbd" && in.Secret == "" {
				return ErrKeyNeed
			}
			if d.Adapter == "qbittorrent" && in.Secret == "" {
				return ErrQbitKey
			}
			if err := dropSecret(tx, &d); err != nil {
				return err
			}
		}
		if d.Adapter == "qbittorrent" && (d.Username != "") != oldLogin && in.Secret == "" {
			return ErrQbitKey
		}
		if hostOf(d.URL) != oldHost && d.SecretRef != nil && in.Secret == "" {
			return ErrKeyAgain
		}
		if err := putSecret(tx, v, &d, in.Secret, by); err != nil {
			return err
		}
		d.BackoffUntil, d.FailCount, d.LastError = nil, 0, ""
		if d.State != "disabled" {
			d.State = "ok"
		}
		return database.Update(tx, "downloaders", &d, by)
	})
	return d, err
}

// Remove deletes the Downloader with the given id.
func Remove(ctx context.Context, s *database.Store, id int64, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		var d domain.Downloader
		if err := database.Get(ctx, tx, "downloaders", id, &d); err != nil {
			return err
		}
		if d.Removed {
			return database.ErrNotFound
		}
		if n, err := pendingAttemptCount(tx, id); err != nil {
			return err
		} else if n > 0 {
			return ErrPendingAttempt
		}
		var all int
		if err := tx.QueryRow("SELECT (SELECT count(*) FROM downloads WHERE downloader_id = ?) + (SELECT count(*) FROM downloader_attempts WHERE downloader_id = ?)", id, id).Scan(&all); err != nil {
			return err
		}
		active, err := activeCount(tx, id)
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrBusy
		}
		if err := dropSecret(tx, &d); err != nil {
			return err
		}
		if all == 0 {
			_, err := tx.Exec("DELETE FROM downloaders WHERE id = ?", id)
			return err
		}
		d.Removed, d.State = true, "disabled"
		d.Name = d.Name + " (removed " + strconv.FormatInt(id, 10) + ")"
		return database.Update(tx, "downloaders", &d, by)
	})
}

// SetPaused pauses or resumes the Downloader with the given id.
func SetPaused(ctx context.Context, s *database.Store, id int64, paused bool, by domain.Actor) (domain.Downloader, error) {
	var d domain.Downloader
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "downloaders", id, &d); err != nil {
			return err
		}
		if d.Removed {
			return database.ErrNotFound
		}
		if paused {
			d.State = "disabled"
		} else {
			d.State, d.BackoffUntil, d.FailCount, d.LastError = "ok", nil, 0, ""
		}
		return database.Update(tx, "downloaders", &d, by)
	})
	return d, err
}

// HealthOf builds the health view of d as of now.
func HealthOf(d domain.Downloader, now time.Time) HealthView {
	switch {
	case d.State == "disabled":
		return HealthView{"paused", "Paused", "Paused by you: HolyArrs sends nothing new to it. Downloads already there are still followed."}
	case d.State == "auth_failed":
		return HealthView{"sign_in_refused", "Sign-in refused", "It turned down the key or sign-in. Check them and test again."}
	case d.BackoffUntil != nil && d.BackoffUntil.After(now):
		return HealthView{"resting", "Resting", "It did not answer, so HolyArrs waits until " + d.BackoffUntil.UTC().Format("15:04 UTC") + " before sending to it again. " + d.LastError}
	case d.State == "unreachable":
		return HealthView{"not_answering", "Not answering", "It did not answer the last check. " + d.LastError}
	}
	return HealthView{"working", "Working", ""}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func absPath(p string) bool {
	return strings.HasPrefix(p, "/") || len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

func fill(in Input, d *domain.Downloader, isNew bool) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	d.Name = name
	if isNew || in.Adapter != "" {
		switch in.Adapter {
		case "qbittorrent", "transmission", "deluge", "rtorrent":
			d.Protocol = "torrent"
		case "sabnzbd", "nzbget":
			d.Protocol = "usenet"
		default:
			return ErrAdapter
		}
		if !isNew && in.Adapter != d.Adapter {
			return ErrAdapter
		}
		d.Adapter = in.Adapter
	}
	if raw := strings.TrimSpace(in.URL); raw != "" || isNew {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" {
			return ErrURL
		}
		d.URL = strings.TrimRight(raw, "/")
	}
	if in.Username != nil {
		if len(*in.Username) > 100 {
			return ErrUsername
		}
		d.Username = strings.TrimSpace(*in.Username)
	}
	if in.Category != nil {
		c, err := checkCategory(*in.Category, d.Adapter)
		if err != nil {
			return err
		}
		if c == "" {
			return ErrCategory
		}
		d.Category = c
	} else if isNew {
		d.Category = "holyarrs"
	}
	for _, k := range []struct {
		in  *string
		out *string
	}{{in.CategoryMovies, &d.CategoryMovies}, {in.CategoryShows, &d.CategoryShows}, {in.CategoryMusic, &d.CategoryMusic}} {
		if k.in == nil {
			continue
		}
		c, err := checkCategory(*k.in, d.Adapter)
		if err != nil {
			return err
		}
		*k.out = c
	}
	if in.Priority != nil {
		if *in.Priority < 1 || *in.Priority > 50 {
			return ErrPriority
		}
		d.Priority = *in.Priority
	} else if isNew {
		d.Priority = 1
	}
	if in.PathMap != nil {
		for _, p := range in.PathMap {
			if !absPath(strings.TrimSpace(p.Theirs)) || !absPath(strings.TrimSpace(p.Ours)) {
				return ErrPathMap
			}
		}
		d.PathMap = in.PathMap
	} else if isNew {
		d.PathMap = []domain.PathMap{}
	}
	if in.AllowLocal != nil {
		d.AllowLocal = *in.AllowLocal
	}
	if in.Writable != nil {
		d.Writable = *in.Writable
	} else if isNew {
		d.Writable = true
	}
	if in.ClearFinished != nil {
		d.ClearFinished = *in.ClearFinished
	} else if isNew {
		d.ClearFinished = true
	}
	if in.Tags != nil || isNew || d.Tags == nil {
		d.Tags = mediafiles.CleanTags(in.Tags)
	}
	if isNew {
		d.AfterImport, d.State = "leave", "ok"
	}
	return nil
}

func nameUsed(tx *sql.Tx, name string, id int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM downloaders WHERE name = ? COLLATE NOCASE AND id != ? AND removed = 0", name, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNameUsed
	}
	return nil
}

func putSecret(tx *sql.Tx, v *secrets.Vault, d *domain.Downloader, plain string, by domain.Actor) error {
	if plain == "" {
		return nil
	}
	if v == nil {
		return ErrNoVault
	}
	if err := dropSecret(tx, d); err != nil {
		return err
	}
	id, err := v.PutTx(tx, "downloader_secret", plain, by)
	d.SecretRef = &id
	return err
}

func dropSecret(tx *sql.Tx, d *domain.Downloader) error {
	if d.SecretRef == nil {
		return nil
	}
	if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", *d.SecretRef); err != nil {
		return err
	}
	d.SecretRef = nil
	return nil
}

func activeCount(tx *sql.Tx, id int64) (int, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM downloads WHERE downloader_id = ? AND state IN ("+activeStates+")", id).Scan(&n)
	return n, err
}

// checkCategory trims a category and checks it; "" is returned as is.
func checkCategory(raw, adapter string) (string, error) {
	c := strings.TrimSpace(raw)
	if c == "" {
		return "", nil
	}
	if len(c) > 64 || strings.ContainsAny(c, `/\`) {
		return "", ErrCategory
	}
	if adapter == "deluge" && strings.Trim(c, "abcdefghijklmnopqrstuvwxyz0123456789_.-") != "" {
		return "", ErrDelugeLabel
	}
	return c, nil
}
