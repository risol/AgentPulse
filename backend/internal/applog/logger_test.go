package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerRotatesAndRedactsUnapprovedAttributes(t *testing.T) {
	directory := t.TempDir()
	logger, err := OpenAt(directory, 180, 2)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		logger.Slog().Error("persistence failed", "error", "private database path and payload", "signal", "logs", "code", "storage_failure")
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	rotated := 0
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if strings.Contains(text, "private database path") {
			t.Fatalf("sensitive attribute was written into %s", entry.Name())
		}
		if strings.HasPrefix(entry.Name(), activeFileName+".") {
			rotated++
		}
	}
	if rotated == 0 {
		t.Fatal("expected at least one rotated log file")
	}
}

func TestCleanupExpiredRemovesOnlyOldRotatedLogs(t *testing.T) {
	directory := t.TempDir()
	active := filepath.Join(directory, activeFileName)
	oldArchive := filepath.Join(directory, activeFileName+".1")
	newArchive := filepath.Join(directory, activeFileName+".2")
	for _, path := range []string{active, oldArchive, newArchive} {
		if err := os.WriteFile(path, []byte("log"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(oldArchive, now.Add(-15*24*time.Hour), now.Add(-15*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := CleanupExpiredAt(directory, now, DefaultRetention)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || result.Bytes != 3 {
		t.Fatalf("cleanup result = %+v, want one 3-byte archive", result)
	}
	for _, path := range []string{active, newArchive} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("newer or active log %s was removed: %v", path, err)
		}
	}
	if _, err := os.Stat(oldArchive); !os.IsNotExist(err) {
		t.Errorf("expired archive still exists: %v", err)
	}
}
