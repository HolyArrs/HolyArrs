package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
)

func (e *Engine) saveManifest(id string, raw []byte) {
	dir := filepath.Join(e.Dir, manifestDir)
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = writeFileSynced(filepath.Join(dir, id+".json"), raw)
	}
}

// listedManifest is the manifest of a listed version, fetched once and
// cached; it counts only when its bytes match the signed list's hash and id.
func (e *Engine) listedManifest(ctx context.Context, en Entry) *Manifest {
	path := filepath.Join(e.Dir, listManifestDir, en.ID+"-"+en.Version+"-"+e.platform()+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if raw, err = e.fetch(ctx, FileBase(en.ID, en.Version, e.platform())+"manifest.json"); err != nil {
			log.Printf("plugins: manifest fetch for %s %s failed: %v", en.ID, en.Version, err)
			return nil
		}
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != en.ManifestSHA256 {
		log.Printf("plugins: manifest for %s %s does not match the list's hash", en.ID, en.Version)
		return nil
	}
	m, err := ParseManifest(raw)
	if err != nil || m.ID != en.ID || m.Version != en.Version {
		log.Printf("plugins: manifest for %s %s is not valid or names another plugin (%v)", en.ID, en.Version, err)
		return nil
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = writeFileSynced(path, raw)
	}
	return m
}

func (e *Engine) readManifest(id string) *Manifest {
	raw, err := os.ReadFile(filepath.Join(e.Dir, manifestDir, id+".json"))
	if err != nil {
		return nil
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil
	}
	return m
}
