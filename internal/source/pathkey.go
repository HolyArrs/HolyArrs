package source

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Some feeds keep their passkey in the address path, not the query
// (https://rss24h.torrentleech.org/<passkey>). Those path parts go to the
// vault as the Source's path secret (the values joined with /); the stored
// address holds a marker in their place, and ConfigFor puts them back.

// pathMark stands in a stored address for a passkey path part; the number
// after it says which value of the path secret.
const pathMark = "HOLYARRSPATHKEY"

// ErrPathKey and the errors after it are said when a masked passkey in the
// address cannot be kept, each with its reason.
var (
	ErrPathKey     = errors.New("type the full address again: its hidden passkey cannot be kept when the host changes")
	ErrPathKeyNone = errors.New("type the full address again: no passkey is stored for its hidden part")
	ErrPathKeyRSS  = errors.New("type the full address again: a hidden passkey is only kept for an rss Source")
)

var errPathKeyGone = errors.New("the Source address needs a passkey that has gone; type the address again")

// looksSecret says whether a path part looks like a passkey: 20 or more
// letters, digits, - and _, with at least one letter and one digit. A part
// with a plain lowercase word of 5 or more letters between its - or _
// (new-movies-2024-...) is a name, not a key.
func looksSecret(part string) bool {
	if len(part) < 20 {
		return false
	}
	letter, digit := false, false
	for _, r := range part {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letter = true
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	if !letter || !digit {
		return false
	}
	for _, w := range strings.FieldsFunc(part, func(r rune) bool { return r == '-' || r == '_' }) {
		if len(w) >= 5 && strings.Trim(w, "abcdefghijklmnopqrstuvwxyz") == "" {
			return false
		}
	}
	return true
}

// splitPath cuts an address into scheme and host, and its path parts.
func splitPath(raw string) (head string, parts []string) {
	i := strings.Index(raw, "://")
	if i < 0 {
		return raw, nil
	}
	j := strings.IndexByte(raw[i+3:], '/')
	if j < 0 {
		return raw, nil
	}
	return raw[:i+3+j], strings.Split(raw[i+3+j+1:], "/")
}

func isMasked(part string) bool {
	return part == secrets.Masked || part == url.PathEscape(secrets.Masked)
}

func hasMasked(raw string) bool {
	return strings.Contains(raw, secrets.Masked) || strings.Contains(raw, url.PathEscape(secrets.Masked))
}

// hidePath swaps the passkeys in an address path for markers and returns
// them. A masked part (as the API shows the address) takes the stored
// value from old, in order; reused says one did.
func hidePath(raw string, old []string) (stored string, keys []string, reused bool, err error) {
	head, parts := splitPath(raw)
	next := 0
	for i, p := range parts {
		switch {
		case isMasked(p):
			if next >= len(old) {
				return "", nil, false, ErrPathKeyNone
			}
			p, reused = old[next], true
			next++
		case !looksSecret(p):
			continue
		}
		parts[i] = pathMark + strconv.Itoa(len(keys))
		keys = append(keys, p)
	}
	if len(keys) == 0 {
		return raw, nil, false, nil
	}
	return head + "/" + strings.Join(parts, "/"), keys, reused, nil
}

// revealPath puts the path secret's values back into a stored address.
func revealPath(stored string, keys []string) (string, error) {
	head, parts := splitPath(stored)
	if !strings.Contains(stored, pathMark) || parts == nil {
		return stored, nil
	}
	for i, p := range parts {
		n, ok := strings.CutPrefix(p, pathMark)
		if !ok {
			continue
		}
		k, err := strconv.Atoi(n)
		if err != nil || k < 0 || k >= len(keys) || keys[k] == "" {
			return "", errPathKeyGone
		}
		parts[i] = keys[k]
	}
	return head + "/" + strings.Join(parts, "/"), nil
}

// MaskURL is a stored Source address as people see it: each passkey path
// part masked.
func MaskURL(stored string) string {
	head, parts := splitPath(stored)
	if !strings.Contains(stored, pathMark) || parts == nil {
		return stored
	}
	for i, p := range parts {
		if strings.HasPrefix(p, pathMark) {
			parts[i] = secrets.Masked
		}
	}
	return head + "/" + strings.Join(parts, "/")
}

func splitKeys(plain string) []string {
	if plain == "" {
		return nil
	}
	return strings.Split(plain, "/")
}

// HidePathKeys moves passkeys an older save left in rss Source addresses
// into the vault, and says how many Sources it changed. It then takes the
// passkeys out of Release links stored before.
func HidePathKeys(ctx context.Context, s *store.Store, v *secrets.Vault) (int, error) {
	if v == nil {
		return 0, nil
	}
	moved := 0
	err := s.Write(ctx, func(tx *sql.Tx) error {
		moved = 0
		srcs, err := store.Select[domain.Source](ctx, tx, "sources", "format = 'rss' AND path_secret_ref IS NULL")
		if err != nil {
			return err
		}
		for _, src := range srcs {
			// A marker an old address already holds was never ours.
			raw := strings.ReplaceAll(src.URL, pathMark, "HOLYARRS-PATHKEY")
			stored, keys, _, err := hidePath(raw, nil)
			if err != nil || len(keys) == 0 {
				continue
			}
			if err := putSecret(tx, v, &src.PathSecretRef, "source_path_key", strings.Join(keys, "/"), domain.System); err != nil {
				return err
			}
			src.URL = stored
			if err := store.Update(tx, "sources", &src, domain.System); err != nil {
				return err
			}
			moved++
		}
		return nil
	})
	if err != nil {
		return moved, err
	}
	return moved, hideReleaseKeys(ctx, s, v)
}

// hideReleaseKeys takes the path passkeys out of a Source's stored Release
// links. A Source whose keys cannot be read keeps its links: they still work.
func hideReleaseKeys(ctx context.Context, s *store.Store, v *secrets.Vault) error {
	srcs, err := store.Select[domain.Source](ctx, s.Read(), "sources", "format = 'rss' AND path_secret_ref IS NOT NULL")
	if err != nil {
		return err
	}
	for _, src := range srcs {
		cfg, err := ConfigFor(ctx, src, v)
		if err != nil || len(cfg.PathKeys) == 0 {
			continue
		}
		keys := secretValues(cfg)
		err = s.Write(ctx, func(tx *sql.Tx) error {
			links := map[int64]string{}
			for _, k := range cfg.PathKeys {
				if k == "" {
					continue
				}
				rows, err := tx.QueryContext(ctx, "SELECT id, link_ref FROM releases WHERE source_id = ? AND instr(link_ref, ?) > 0", src.ID, k)
				if err != nil {
					return err
				}
				for rows.Next() {
					var id int64
					var link string
					if err := rows.Scan(&id, &link); err != nil {
						rows.Close()
						return err
					}
					links[id] = link
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
			}
			for id, link := range links {
				if clean := hideKeys(link, keys); clean != link {
					if _, err := tx.ExecContext(ctx, "UPDATE releases SET link_ref = ? WHERE id = ?", clean, id); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
