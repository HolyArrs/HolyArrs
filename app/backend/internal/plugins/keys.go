package plugins

import (
	"crypto/ed25519"
	"encoding/base64"
	"log"
)

// pinnedKeys are the ed25519 public keys (base64) the plugin lists must be
// signed with: the main key, then the backup key kept offline. The app and
// the signing tool both read them from here, so they never drift apart.
var pinnedKeys = []string{
	"BFcI/1RFf6k7cRqXaj+YYQsfZu1MQWmiFYktV2QIBHE=", // main
	"ENa3RVFV9VfQgZ/uyaKItB9xmniR36sWM17HWXi+llY=", // backup
}

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
