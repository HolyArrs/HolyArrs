package accounts

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen = 16
	keyLen  = 32
	// MinPassword is the hard floor for a password's length in bytes; the
	// owner's min_password_length setting can only raise it, up to
	// MaxMinPassword. DefaultMinPassword applies until the owner picks one.
	// MaxPassword is the longest password taken.
	MinPassword        = 4
	DefaultMinPassword = 8
	MaxMinPassword     = 64
	MaxPassword        = 1024
	// keyMinLength is the sign-in setting with the owner's minimum length.
	keyMinLength = "min_password_length"
)

var (
	normalHash = hashParams{memKiB: 19 * 1024, passes: 2, lanes: 1}
	testHash   = hashParams{memKiB: 64, passes: 1, lanes: 1}
)

// argon2id settings: the OWASP minimum (19 MiB, 2 passes, 1 lane) keeps a
// sign-in cheap enough for a small NAS while still slow to guess.
type hashParams struct {
	memKiB uint32
	passes uint32
	lanes  uint8
}

// MinPasswordLength reads the owner's minimum password length from the
// settings, clamped to MinPassword..MaxMinPassword; DefaultMinPassword when
// unset or unreadable.
func MinPasswordLength(ctx context.Context, db *sql.DB) int {
	v := database.SettingOr(ctx, db, settingsGroup, signInPrefix+keyMinLength, "")
	n, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMinPassword
	}
	if n < MinPassword {
		return MinPassword
	}
	return min(n, MaxMinPassword)
}

// HashPassword returns an argon2id hash in the usual
// $argon2id$v=19$m=..,t=..,p=..$salt$key form.
func HashPassword(pw string) (string, error) { return hashPassword(pw, normalHash) }

// CheckPassword reports whether pw matches the stored hash.
func CheckPassword(hash, pw string) bool {
	p, salt, key, err := decodeHash(hash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, p.passes, p.memKiB, p.lanes, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

// passwordProblem says what is wrong with a password's length, or "".
func passwordProblem(pw string, minLen int) string {
	if len(pw) < minLen {
		return fmt.Sprintf("the password needs at least %d characters", minLen)
	}
	if len(pw) > MaxPassword {
		return fmt.Sprintf("the password can have at most %d characters", MaxPassword)
	}
	return ""
}

func hashPassword(pw string, p hashParams) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, p.passes, p.memKiB, p.lanes, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.memKiB, p.passes, p.lanes,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func decodeHash(h string) (hashParams, []byte, []byte, error) {
	var p hashParams
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, errors.New("not an argon2id hash")
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return p, nil, nil, errors.New("unknown argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memKiB, &p.passes, &p.lanes); err != nil {
		return p, nil, nil, err
	}
	if p.memKiB == 0 || p.memKiB > 1<<20 || p.passes == 0 || p.passes > 16 || p.lanes == 0 {
		return p, nil, nil, errors.New("argon2 settings out of range")
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, err
	}
	key, err := enc.DecodeString(parts[5])
	if err != nil || len(key) < 16 {
		return p, nil, nil, errors.New("bad key")
	}
	return p, salt, key, nil
}
