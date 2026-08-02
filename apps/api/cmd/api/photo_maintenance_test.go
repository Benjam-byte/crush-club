package main

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveStaleOrphanPhotoFiles(t *testing.T) {
	storagePath := t.TempDir()
	referencedName := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA.jpg"
	staleOrphanName := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB.png"
	recentOrphanName := "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC.webp"
	temporaryName := ".photo-upload-stale"
	unrelatedName := "keep-me.txt"
	for _, name := range []string{referencedName, staleOrphanName, recentOrphanName, temporaryName, unrelatedName} {
		if err := os.WriteFile(filepath.Join(storagePath, name), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	staleTime := now.Add(-2 * time.Hour)
	for _, name := range []string{referencedName, staleOrphanName, temporaryName, unrelatedName} {
		if err := os.Chtimes(filepath.Join(storagePath, name), staleTime, staleTime); err != nil {
			t.Fatal(err)
		}
	}

	removed, failures, err := removeStaleOrphanPhotoFiles(
		storagePath,
		map[string]struct{}{referencedName: {}},
		now.Add(-time.Hour),
		os.Remove,
	)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 || failures != 0 {
		t.Fatalf("removed = %d, failures = %d, want 2, 0", removed, failures)
	}
	for _, name := range []string{referencedName, recentOrphanName, unrelatedName} {
		if _, err := os.Stat(filepath.Join(storagePath, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	for _, name := range []string{staleOrphanName, temporaryName} {
		if _, err := os.Stat(filepath.Join(storagePath, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed, stat error = %v", name, err)
		}
	}
}

func TestRemoveStaleOrphanPhotoFilesReportsRemovalFailure(t *testing.T) {
	storagePath := t.TempDir()
	name := "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD.jpg"
	path := filepath.Join(storagePath, name)
	if err := os.WriteFile(path, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	removed, failures, err := removeStaleOrphanPhotoFiles(
		storagePath,
		map[string]struct{}{},
		time.Now().Add(-time.Hour),
		func(string) error { return os.ErrPermission },
	)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || failures != 1 {
		t.Fatalf("removed = %d, failures = %d, want 0, 1", removed, failures)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed removal should leave the file for a retry: %v", err)
	}
}

func TestRemoveStoredPhotoFilesCanRetryAfterFailure(t *testing.T) {
	storagePath := t.TempDir()
	name := "EEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE.png"
	path := filepath.Join(storagePath, name)
	if err := os.WriteFile(path, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeStoredPhotoFiles(storagePath, []string{name}, func(string) error {
		return os.ErrPermission
	}); err == nil {
		t.Fatal("expected the first removal attempt to fail")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed removal should keep the file for retry: %v", err)
	}
	if err := removeStoredPhotoFiles(storagePath, []string{name}, os.Remove); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry did not remove the file: %v", err)
	}
}

func TestPositiveEnvDuration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("TEST_DURATION", "7")
	if got := positiveEnvDuration(logger, "TEST_DURATION", time.Minute, time.Hour); got != 7*time.Minute {
		t.Fatalf("duration = %v, want 7m", got)
	}
	t.Setenv("TEST_DURATION", "invalid")
	if got := positiveEnvDuration(logger, "TEST_DURATION", time.Minute, time.Hour); got != time.Hour {
		t.Fatalf("invalid duration = %v, want fallback 1h", got)
	}
}
