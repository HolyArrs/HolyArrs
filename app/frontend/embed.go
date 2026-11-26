// Package frontend embeds the built web app so the Go server can serve it.
package frontend

import (
	"bytes"
	"embed"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

//go:embed all:dist
var files embed.FS

// Files is the built app, rooted at its folder. The build keeps most scripts
// and styles only as brotli copies (name.br) to keep the program small. Files
// hands them out under their plain names, unpacked.
var Files fs.FS = packed{mustSub(files, "dist")}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// packed serves name from name.br when name itself is not there.
type packed struct{ fs.FS }

func (p packed) Open(name string) (fs.File, error) {
	f, err := p.FS.Open(name)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || strings.HasSuffix(name, ".br") {
		return f, err
	}
	var b []byte
	if z, zerr := p.FS.Open(name + ".br"); zerr == nil {
		defer z.Close()
		b, err = io.ReadAll(brotli.NewReader(z))
	} else {
		return nil, err
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &unpacked{Reader: bytes.NewReader(b), name: name[strings.LastIndex(name, "/")+1:], size: int64(len(b))}, nil
}

// unpacked is one file read out of its packed copy.
type unpacked struct {
	*bytes.Reader
	name string
	size int64
}

func (u *unpacked) Stat() (fs.FileInfo, error) { return u, nil }
func (u *unpacked) Close() error               { return nil }
func (u *unpacked) Name() string               { return u.name }
func (u *unpacked) Size() int64                { return u.size }
func (u *unpacked) Mode() fs.FileMode          { return 0o444 }
func (u *unpacked) ModTime() time.Time         { return time.Time{} }
func (u *unpacked) IsDir() bool                { return false }
func (u *unpacked) Sys() any                   { return nil }
