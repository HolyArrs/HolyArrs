package plugins

import (
	"crypto/ed25519"
	"encoding/base64"
	"log"
)

// pinnedKeys are the ed25519 public keys (base64) the plugin lists must be
// signed with. The main key lives in the plugins repo's signing environment,
// with an offline copy the maintainer keeps. The backup key is kept offline only and
// signs if the main key is ever lost (docs/plugin-signing-key.md). The app and
// the signing tool both read them from here, so they never drift apart.
var pinnedKeys = []string{
	"n5a8nBqYlumtATVUUeOUYSWgbWI/yi62H1xDhXx6jjY=", // main
	"dmodZ/Y7L1QCT4EqPgZCxDhe1ZpWz1xhCgnl6PJ/uNQ=", // backup
}

// PinnedKeyStrings returns the pinned keys as written (base64).
func PinnedKeyStrings() []string { return append([]string(nil), pinnedKeys...) }

// PinnedKeys returns the pinned public keys, skipping any that is malformed.
func PinnedKeys() []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	for _, k := range pinnedKeys {
		b, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(b) != ed25519.PublicKeySize {
			log.Printf("plugins: pinned key %q is not an ed25519 public key; skipped", k)
			continue
		}
		keys = append(keys, ed25519.PublicKey(b))
	}
	return keys
}
