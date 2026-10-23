package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (s *SQLite) SchemaVersion() int {
	return schemaVersion
}

func (s *SQLite) LoadTelemetryRetentionDays(ctx context.Context) (int, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key='telemetry_retention_days'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return telemetry.DefaultTelemetryRetentionDays, nil
	}
	if err != nil {
		return 0, err
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 365 {
		return 0, fmt.Errorf("stored telemetry retention setting is invalid")
	}
	return days, nil
}

func (s *SQLite) SaveTelemetryRetentionDays(ctx context.Context, days int) error {
	if days < 1 || days > 365 {
		return fmt.Errorf("telemetry retention must be between 1 and 365 days")
	}
	now := time.Now().UnixNano()
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_state(key,value,updated_at_ns) VALUES('telemetry_retention_days',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at_ns=excluded.updated_at_ns`, strconv.Itoa(days), now)
	return err
}

func (s *SQLite) LoadReceiverPort(ctx context.Context) (*int, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key='receiver_port'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("stored receiver port is invalid: %w", err)
	}
	return &port, nil
}

func (s *SQLite) SaveReceiverPort(ctx context.Context, port int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_state(key,value,updated_at_ns) VALUES('receiver_port',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at_ns=excluded.updated_at_ns`, strconv.Itoa(port), time.Now().UnixNano())
	return err
}

func (s *SQLite) RecordIngestDiagnostic(ctx context.Context, signal telemetry.Signal, code string) error {
	if signal != telemetry.SignalLogs && signal != telemetry.SignalMetrics && signal != telemetry.SignalTraces {
		return fmt.Errorf("invalid ingest diagnostic signal")
	}
	switch code {
	case "unsupported_content_type", "request_too_large", "invalid_request_body", "invalid_protobuf", "storage_failure":
	default:
		return fmt.Errorf("invalid ingest diagnostic code")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ingest_diagnostics(signal_type,error_code,occurred_at_ns) VALUES(?,?,?)`, string(signal), code, time.Now().UnixNano()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ingest_diagnostics WHERE id NOT IN (SELECT id FROM ingest_diagnostics ORDER BY occurred_at_ns DESC,id DESC LIMIT 200)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) ListRecentIngestDiagnostics(ctx context.Context, limit int) ([]telemetry.IngestDiagnostic, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid ingest diagnostic limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,signal_type,error_code,occurred_at_ns FROM ingest_diagnostics ORDER BY occurred_at_ns DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.IngestDiagnostic, 0, limit)
	for rows.Next() {
		var item telemetry.IngestDiagnostic
		if err := rows.Scan(&item.ID, &item.Signal, &item.Code, &item.OccurredAtNS); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) CountIngestDiagnosticsSince(ctx context.Context, sinceNS int64) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ingest_diagnostics WHERE occurred_at_ns>=?`, sinceNS).Scan(&count)
	return count, err
}

// PruneTelemetryHistoryBatch removes a bounded number of expired rows in one
// transaction so ingestion can resume between cleanup batches.
func (s *SQLite) PruneTelemetryHistoryBatch(ctx context.Context, cutoffNS int64, batchSize int) (int64, error) {
	if cutoffNS <= 0 || batchSize < 1 || batchSize > 5000 {
		return 0, fmt.Errorf("invalid telemetry cleanup bounds")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var deleted int64
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE sessions SET data_quality='partial' WHERE session_id IN (
  SELECT session_id FROM (
    SELECT session_id FROM telemetry_events WHERE received_at_ns<? AND session_id IS NOT NULL
    UNION SELECT session_id FROM spans WHERE received_at_ns<? AND session_id IS NOT NULL
  ) LIMIT ?
)`, []any{cutoffNS, cutoffNS, batchSize}},
		{`DELETE FROM tool_calls WHERE rowid IN (
  SELECT tc.rowid FROM tool_calls tc WHERE
    ((tc.start_time_ns IS NOT NULL AND tc.start_time_ns<?) OR
     tc.session_id IN (SELECT session_id FROM sessions WHERE COALESCE(last_activity_ns,created_at_ns)<?) OR
     tc.source_event_id IN (SELECT id FROM telemetry_events WHERE received_at_ns<?))
    AND NOT EXISTS (SELECT 1 FROM telemetry_events e WHERE e.id=tc.source_event_id AND e.received_at_ns>=?)
    AND NOT EXISTS (SELECT 1 FROM spans sp WHERE sp.trace_id=tc.trace_id AND sp.span_id=tc.span_id AND sp.received_at_ns>=?)
  ORDER BY COALESCE(tc.start_time_ns,0),tc.rowid LIMIT ?
)`, []any{cutoffNS, cutoffNS, cutoffNS, cutoffNS, cutoffNS, batchSize}},
		{`DELETE FROM turns WHERE turn_id IN (
  SELECT t.turn_id FROM turns t WHERE
    (COALESCE(t.last_activity_ns,t.started_at_ns,0)<? OR
     t.session_id IN (SELECT session_id FROM sessions WHERE COALESCE(last_activity_ns,created_at_ns)<?))
    AND NOT EXISTS (SELECT 1 FROM telemetry_events e WHERE e.turn_id=t.turn_id AND e.received_at_ns>=?)
    AND NOT EXISTS (SELECT 1 FROM spans sp WHERE sp.turn_id=t.turn_id AND sp.received_at_ns>=?)
  ORDER BY COALESCE(t.last_activity_ns,t.started_at_ns,0),t.turn_id LIMIT ?
)`, []any{cutoffNS, cutoffNS, cutoffNS, cutoffNS, batchSize}},
		{`DELETE FROM sessions WHERE session_id IN (
  SELECT s.session_id FROM sessions s WHERE COALESCE(s.last_activity_ns,s.created_at_ns)<?
    AND NOT EXISTS (SELECT 1 FROM telemetry_events e WHERE e.session_id=s.session_id AND e.received_at_ns>=?)
    AND NOT EXISTS (SELECT 1 FROM spans sp WHERE sp.session_id=s.session_id AND sp.received_at_ns>=?)
  ORDER BY COALESCE(s.last_activity_ns,s.created_at_ns),s.session_id LIMIT ?
)`, []any{cutoffNS, cutoffNS, cutoffNS, batchSize}},
		{`DELETE FROM spans WHERE rowid IN (
  SELECT rowid FROM spans WHERE received_at_ns<? ORDER BY received_at_ns,rowid LIMIT ?
)`, []any{cutoffNS, batchSize}},
		{`DELETE FROM telemetry_events WHERE id IN (
  SELECT id FROM telemetry_events WHERE received_at_ns<? ORDER BY received_at_ns,id LIMIT ?
)`, []any{cutoffNS, batchSize}},
		{`DELETE FROM metric_points WHERE id IN (
  SELECT id FROM metric_points WHERE received_at_ns<? ORDER BY received_at_ns,id LIMIT ?
)`, []any{cutoffNS, batchSize}},
	}
	for _, statement := range statements {
		result, err := tx.ExecContext(ctx, statement.query, statement.args...)
		if err != nil {
			return 0, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += rows
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *SQLite) DatabaseSizeBytes(ctx context.Context) (int64, error) {
	var pageCount, pageSize int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}

func (s *SQLite) OptimizeDatabase(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	return err
}

func (s *SQLite) CreateDatabaseSnapshot(ctx context.Context) (io.ReadCloser, int64, error) {
	directory, err := os.MkdirTemp("", "agentpulse-export-")
	if err != nil {
		return nil, 0, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "agentpulse-data.db")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		cleanup()
		return nil, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		cleanup()
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, err
	}
	return &temporarySnapshot{File: file, cleanup: cleanup}, info.Size(), nil
}

func (s *SQLite) ClearHistory(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var deleted int64
	for _, table := range []string{
		"tool_calls", "turns", "sessions", "spans", "telemetry_events", "metric_points", "process_samples", "processes",
	} {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+table)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += count
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

type temporarySnapshot struct {
	*os.File
	cleanup func()
}

func (s *temporarySnapshot) Close() error {
	closeErr := s.File.Close()
	s.cleanup()
	return closeErr
}

func (s *SQLite) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLite) LastReceivedTimes(ctx context.Context) (map[telemetry.Signal]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM app_state WHERE key IN (?,?,?)`,
		"last_received_"+string(telemetry.SignalLogs)+"_ns",
		"last_received_"+string(telemetry.SignalMetrics)+"_ns",
		"last_received_"+string(telemetry.SignalTraces)+"_ns")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[telemetry.Signal]int64, 3)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		receivedAt, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		for _, signal := range []telemetry.Signal{telemetry.SignalLogs, telemetry.SignalMetrics, telemetry.SignalTraces} {
			if key == "last_received_"+string(signal)+"_ns" {
				result[signal] = receivedAt
				break
			}
		}
	}
	return result, rows.Err()
}

func (s *SQLite) CountEventsSince(ctx context.Context, since int64) (map[telemetry.Signal]int64, error) {
	result := make(map[telemetry.Signal]int64, 3)
	for _, signal := range []telemetry.Signal{telemetry.SignalLogs, telemetry.SignalMetrics, telemetry.SignalTraces} {
		var count int64
		query := `SELECT count(*) FROM telemetry_events WHERE signal_type=? AND received_at_ns>=?`
		if signal == telemetry.SignalMetrics {
			query = `SELECT count(*) FROM metric_points WHERE received_at_ns>=?`
			if err := s.db.QueryRowContext(ctx, query, since).Scan(&count); err != nil {
				return nil, err
			}
			result[signal] = count
			continue
		}
		if err := s.db.QueryRowContext(ctx, query, string(signal), since).Scan(&count); err != nil {
			return nil, err
		}
		result[signal] = count
	}
	return result, nil
}
