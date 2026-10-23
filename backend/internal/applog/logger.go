// Package applog provides bounded, local-only logs with an attribute allowlist.
package applog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxBytes      = 5 << 20
	DefaultMaxFiles      = 3
	DefaultRetention     = 14 * 24 * time.Hour
	activeFileName       = "agentpulse.log"
	minimumRotationFiles = 1
)

type CleanupResult struct {
	Files int   `json:"deleted_files"`
	Bytes int64 `json:"deleted_bytes"`
}

type Logger struct {
	mu        sync.Mutex
	directory string
	file      *os.File
	size      int64
	maxBytes  int64
	maxFiles  int
	closed    bool
	logger    *slog.Logger
}

func OpenDefault() (*Logger, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(configDirectory, "AgentPulse", "logs"), DefaultMaxBytes, DefaultMaxFiles)
}

func OpenAt(directory string, maxBytes int64, maxFiles int) (*Logger, error) {
	if maxBytes < 1 || maxFiles < minimumRotationFiles {
		return nil, errors.New("invalid application log bounds")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	if _, err := CleanupExpiredAt(directory, time.Now(), DefaultRetention); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, activeFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	logger := &Logger{directory: directory, file: file, size: info.Size(), maxBytes: maxBytes, maxFiles: maxFiles}
	handler := &safeHandler{next: slog.NewJSONHandler(logger, &slog.HandlerOptions{Level: slog.LevelInfo})}
	logger.logger = slog.New(handler)
	return logger, nil
}

func (l *Logger) Slog() *slog.Logger { return l.logger }

func (l *Logger) Directory() string { return l.directory }

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if err := l.file.Sync(); err != nil {
		_ = l.file.Close()
		return err
	}
	return l.file.Close()
}

func (l *Logger) CleanupExpired(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	result, err := CleanupExpiredAt(l.directory, time.Now(), DefaultRetention)
	return result.Bytes, err
}

func CleanupExpiredAt(directory string, now time.Time, retention time.Duration) (CleanupResult, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return CleanupResult{}, nil
	}
	if err != nil {
		return CleanupResult{}, err
	}
	cutoff := now.Add(-retention)
	var result CleanupResult
	for _, entry := range entries {
		if entry.IsDir() || !isRotatedLog(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return result, err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return result, err
		}
		result.Files++
		result.Bytes += info.Size()
	}
	return result, nil
}

func (l *Logger) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	if l.size > 0 && l.size+int64(len(data)) > l.maxBytes {
		if err := l.rotateLocked(); err != nil {
			return 0, err
		}
	}
	written, err := l.file.Write(data)
	l.size += int64(written)
	return written, err
}

func (l *Logger) rotateLocked() error {
	if err := l.file.Sync(); err != nil {
		return err
	}
	if err := l.file.Close(); err != nil {
		return err
	}
	for index := l.maxFiles; index >= 1; index-- {
		source := filepath.Join(l.directory, activeFileName+"."+strconv.Itoa(index))
		destination := filepath.Join(l.directory, activeFileName+"."+strconv.Itoa(index+1))
		if index == l.maxFiles {
			if err := os.Remove(source); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(source, destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	active := filepath.Join(l.directory, activeFileName)
	if err := os.Rename(active, filepath.Join(l.directory, activeFileName+".1")); err != nil {
		return err
	}
	file, err := os.OpenFile(active, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	l.file = file
	l.size = 0
	return nil
}

func isRotatedLog(name string) bool {
	if !strings.HasPrefix(name, activeFileName+".") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimPrefix(name, activeFileName+"."))
	return err == nil
}

type safeHandler struct{ next slog.Handler }

func (h *safeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *safeHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		if safeLogAttribute(attribute) {
			safe.AddAttrs(attribute)
		}
		return true
	})
	return h.next.Handle(ctx, safe)
}

func (h *safeHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	filtered := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		if safeLogAttribute(attribute) {
			filtered = append(filtered, attribute)
		}
	}
	return &safeHandler{next: h.next.WithAttrs(filtered)}
}

func (h *safeHandler) WithGroup(name string) slog.Handler {
	// Group names can be data too. The application currently has no grouped
	// logging contract, so ignore groups rather than persisting arbitrary names.
	return h
}

func safeLogAttribute(attribute slog.Attr) bool {
	attribute.Value = attribute.Value.Resolve()
	switch attribute.Key {
	case "signal":
		value := attribute.Value.String()
		return value == "logs" || value == "metrics" || value == "traces"
	case "code":
		switch attribute.Value.String() {
		case "unsupported_content_type", "request_too_large", "invalid_request_body", "invalid_protobuf", "storage_failure":
			return true
		}
		return false
	case "state":
		switch attribute.Value.String() {
		case "ready", "stale", "no_data", "stopping", "failed":
			return true
		}
		return false
	case "schema_version", "duration_ms", "records", "deleted_rows":
		return attribute.Value.Kind() == slog.KindInt64 || attribute.Value.Kind() == slog.KindUint64 || attribute.Value.Kind() == slog.KindFloat64
	case "database_code":
		return attribute.Value.Kind() == slog.KindInt64 && attribute.Value.Int64() >= 0 && attribute.Value.Int64() <= 32767
	case "storage_error_class":
		switch attribute.Value.String() {
		case "deadline", "canceled", "sqlite_busy", "sqlite_interrupted", "sqlite_io", "sqlite_constraint", "sqlite", "other":
			return true
		}
		return false
	default:
		return false
	}
}

var _ io.WriteCloser = (*Logger)(nil)
var _ slog.Handler = (*safeHandler)(nil)
