// Package signtest makes throwaway minisign keys and signatures for tests.
// Keys are made fresh in each test; no private key is ever stored.
package signtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"golang.org/x/crypto/blake2b"
)

// Key is a test signing key.
type Key struct {
	ID   [8]byte
	Priv ed25519.PrivateKey
	Pub  updates.PublicKey
}

// New makes a fresh key.
func New(t testing.TB) Key {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	return Key{ID: id, Priv: priv, Pub: updates.PublicKey{ID: id, Key: pub}}
}

// PublicLine is the key in minisign's public key format.
func (k Key) PublicLine() string {
	return base64.StdEncoding.EncodeToString(append(append([]byte("Ed"), k.ID[:]...), k.Pub.Key...))
}

// Sign makes a minisign "ED" signature (prehashed with BLAKE2b-512).
func (k Key) Sign(data []byte, trusted string) []byte { return k.sign(data, trusted, true) }

// SignLegacy makes a legacy "Ed" signature.
func (k Key) SignLegacy(data []byte, trusted string) []byte { return k.sign(data, trusted, false) }

func (k Key) sign(data []byte, trusted string, hashed bool) []byte {
	alg, msg := "Ed", data
	if hashed {
		h := blake2b.Sum512(data)
		alg, msg = "ED", h[:]
	}
	s := ed25519.Sign(k.Priv, msg)
	raw := append(append([]byte(alg), k.ID[:]...), s...)
	global := ed25519.Sign(k.Priv, append(append([]byte{}, s...), trusted...))
	return []byte("untrusted comment: signature from test key\n" +
		base64.StdEncoding.EncodeToString(raw) + "\n" +
		"trusted comment: " + trusted + "\n" +
		base64.StdEncoding.EncodeToString(global) + "\n")
}
