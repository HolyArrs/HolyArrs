package library

import (
	"context"
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestRemoveFileProtectedWithAckRefused(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	runSQL(t, s, "UPDATE files SET protected = 1 WHERE id = ?", fs[0].ID)
	_, err := svc.RemoveFileOf(context.Background(), id, fs[0].ID, RemoveChoice{DeleteFiles: true, AcknowledgeProtection: true}, true, domain.System)
	if !errors.Is(err, ErrProtectedFile) || !onDisk(lf, fs[0].RelativePath) {
		t.Fatalf("protected+ack: err=%v onDisk=%v", err, onDisk(lf, fs[0].RelativePath))
	}
}

func TestRemoveFileNoDeleteFiles(t *testing.T) {
	svc, _, lf, id, fs := deleteWorld(t)
	_, err := svc.RemoveFileOf(context.Background(), id, fs[0].ID, RemoveChoice{}, true, domain.System)
	var kept ErrFileKept
	if !errors.As(err, &kept) || !onDisk(lf, fs[0].RelativePath) {
		t.Fatalf("delete_files false: %v", err)
	}
}

func TestRemoveFileOtherTitlesFile(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	other := dune
	other.Name, other.IDs = "Other", map[string]string{"tmdb": "1"}
	id2, err := AddMovie(context.Background(), s, other, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_ = id
	_, err = svc.RemoveFileOf(context.Background(), id2, fs[0].ID, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if !errors.Is(err, ErrNoSuchFile) || !onDisk(lf, fs[0].RelativePath) {
		t.Fatalf("other title: %v", err)
	}
}

func TestRemoveFileMissing(t *testing.T) {
	svc, s, _, id, fs := deleteWorld(t)
	runSQL(t, s, "UPDATE files SET state = 'missing' WHERE id = ?", fs[0].ID)
	_, err := svc.RemoveFileOf(context.Background(), id, fs[0].ID, RemoveChoice{DeleteFiles: true}, true, domain.System)
	var kept ErrFileKept
	if !errors.As(err, &kept) {
		t.Fatalf("missing: %v", err)
	}
}
