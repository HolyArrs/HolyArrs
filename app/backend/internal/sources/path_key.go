package sources

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Some feeds keep their passkey in the address path instead of the query, as in
// https://rss24h.torrentleech.org/<passkey>. Those path parts go to the vault as the Source's path
// secret, with the values joined by /. The stored address holds a marker in their place, and ConfigFor
// puts the values back.
// pathMark is that marker. The number after it says which value of the path secret to use.
const pathMark = "HOLYARRSPATHKEY"

// Errors for a masked passkey in the address that cannot be kept. Each message says why.
var (
	ErrPathKey     = errors.New("type the full address again: its hidden passkey cannot be kept when the host changes")
	ErrPathKeyNone = errors.New("type the full address again: no passkey is stored for its hidden part")
	ErrPathKeyRSS  = errors.New("type the full address again: a hidden passkey is only kept for an rss Source")
)

// errPathKeyGone is returned when a stored marker points at a passkey that no longer exists.
var errPathKeyGone = errors.New("the Source address needs a passkey that has gone; type the address again")

// MaskURL returns a stored Source address as people see it, with each passkey path part masked.
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

// HidePathKeys moves passkeys that an older save left in rss Source addresses into the vault.
// It returns how many Sources it changed, then takes the passkeys out of Release links stored before.
func HidePathKeys(ctx context.Context, s *database.Store, v *secrets.Vault) (int, error) {
	if v == nil {
		return 0, nil
	}
	moved := 0
	err := s.Write(ctx, func(tx *sql.Tx) error {
		moved = 0
		srcs, err := database.Select[domain.Source](ctx, tx, "sources", "format = 'rss' AND path_secret_ref IS NULL")
		if err != nil {
			return err
		}
		for _, src := range srcs {
			// A marker that an old address already holds was never ours.
			raw := strings.ReplaceAll(src.URL, pathMark, "HOLYARRS-PATHKEY")
			stored, keys, _, err := hidePath(raw, nil)
			if err != nil || len(keys) == 0 {
				continue
			}
			if err := putSecret(tx, v, &src.PathSecretRef, "source_path_key", strings.Join(keys, "/"), domain.System); err != nil {
				return err
			}
			src.URL = stored
			if err := database.Update(tx, "sources", &src, domain.System); err != nil {
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

// looksSecret says whether a path part looks like a passkey.
// It needs 20 or more letters, digits, - and _, with at least one letter and one digit.
// A plain lowercase word of 5 or more letters between - or _ marks a name such as new-movies-2024, not a key.
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

// splitPath cuts an address into its scheme and host, and its path parts.
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

// isMasked says whether a path part is the mask, plain or escaped.
func isMasked(part string) bool {
	return part == secrets.Masked || part == url.PathEscape(secrets.Masked)
}

// hasMasked says whether an address holds the mask anywhere.
func hasMasked(raw string) bool {
	return strings.Contains(raw, secrets.Masked) || strings.Contains(raw, url.PathEscape(secrets.Masked))
}

// hidePath swaps the passkeys in an address path for markers and returns them.
// A masked part, as the API shows the address, takes the stored value from old in order.
// reused says that at least one did.
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

// revealPath puts the values of the path secret back into a stored address.
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

// splitKeys splits the stored path secret into its values.
func splitKeys(plain string) []string {
	if plain == "" {
		return nil
	}
	return strings.Split(plain, "/")
}

// hideReleaseKeys takes the path passkeys out of a Source's stored Release links.
// A Source whose keys cannot be read keeps its links, because they still work.
func hideReleaseKeys(ctx context.Context, s *database.Store, v *secrets.Vault) error {
	srcs, err := database.Select[domain.Source](ctx, s.Read(), "sources", "format = 'rss' AND path_secret_ref IS NOT NULL")
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
				if err := collectLinksWith(ctx, tx, src.ID, k, links); err != nil {
					return err
				}
			}
			for id, link := range links {
				if clean := hideKeys(link, keys); clean != link {
					if err := setReleaseLink(ctx, tx, id, clean); err != nil {
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
