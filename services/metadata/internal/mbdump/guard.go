package mbdump

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// CapBytes is the music data cap: slim copies (new and old, with their
// indexes) plus the music pictures.
const CapBytes int64 = 90 << 30

// BuildCapBytes is the cap while a refresh builds the new copy beside the
// old one. After the swap the old copy goes and the data is back under
// CapBytes.
const BuildCapBytes int64 = 100 << 30

// ErrOverCap: the music data would pass the cap; stop and ask.
var ErrOverCap = errors.New("mbdump: music data over the cap")

// Guard counts the slim copies on disk plus the music picture bytes. It
// never touches the picture store's own cap.
type Guard struct {
	Limit      int64                                    // 0: CapBytes
	BuildLimit int64                                    // 0: BuildCapBytes
	Root       string                                   // every copy lives under it
	Pictures   func(ctx context.Context) (int64, error) // music picture bytes (artist, album, wide, logo)
}

// Check answers the bytes counted, with ErrOverCap past the limit.
func (g *Guard) Check(ctx context.Context) (int64, error) {
	limit := g.Limit
	if limit <= 0 {
		limit = CapBytes
	}
	return g.check(ctx, limit)
}

// CheckBuild is Check with the build-time cap, for use while a new copy is
// being built.
func (g *Guard) CheckBuild(ctx context.Context) (int64, error) {
	limit := g.BuildLimit
	if limit <= 0 {
		limit = BuildCapBytes
	}
	return g.check(ctx, limit)
}

func (g *Guard) check(ctx context.Context, limit int64) (int64, error) {
	var used int64
	if g.Root != "" {
		var err error
		if used, err = DirBytes(g.Root); err != nil {
			return 0, err
		}
	}
	if g.Pictures != nil {
		p, err := g.Pictures(ctx)
		if err != nil {
			return used, err
		}
		used += p
	}
	if used > limit {
		return used, fmt.Errorf("%w: %d of %d bytes", ErrOverCap, used, limit)
	}
	return used, nil
}

// DirBytes answers the bytes the files under dir take (0 when missing).
func DirBytes(dir string) (int64, error) {
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n, err
}
