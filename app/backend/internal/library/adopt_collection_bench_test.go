package library

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCollectionSharedFolderDiscovery(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 200; i++ {
		dir := filepath.Join(root, fmt.Sprintf("Movie %d (2001)", i))
		if err := os.Mkdir(dir, 0755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("video"), 0644); err != nil {
			b.Fatal(err)
		}
	}
	shared := map[string]bool{}
	for i := 0; i < 10000; i++ {
		shared[fmt.Sprintf("Other %d/Movies", i)] = true
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(movieCandidates(root, ents, shared)) != 200 {
			b.Fatal("wrong candidate count")
		}
	}
}
