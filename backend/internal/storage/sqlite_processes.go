package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/agentpulse/agentpulse/backend/internal/correlation"
	"github.com/agentpulse/agentpulse/backend/internal/process"
	"time"
)

func (s *SQLite) LoadProcessMonitorConfig(ctx context.Context) (process.Config, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key='process_monitor_config'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return process.DefaultConfig(), nil
	}
	if err != nil {
		return process.Config{}, err
	}
	var config process.Config
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return process.Config{}, err
	}
	return process.ValidateConfig(config)
}

func (s *SQLite) SaveProcessMonitorConfig(ctx context.Context, config process.Config) error {
	config, err := process.ValidateConfig(config)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_state(key,value,updated_at_ns) VALUES('process_monitor_config',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at_ns=excluded.updated_at_ns`, string(encoded), time.Now().UnixNano())
	return err
}

func (s *SQLite) ApplyProcessSnapshot(ctx context.Context, snapshot process.Snapshot, changes process.ChangeSet) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sample := range snapshot.Processes {
		if sample.ProcessID == "" {
			sample.ProcessID = process.InstanceID(sample.PID, sample.StartedAtNS)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO processes(process_id,pid,parent_pid,image_name,executable_path,command_line,started_at_ns,ended_at_ns,exit_code,created_at_ns) VALUES(?,?,?,?,?,?,?,NULL,NULL,?) ON CONFLICT(process_id) DO UPDATE SET parent_pid=excluded.parent_pid,image_name=excluded.image_name,executable_path=COALESCE(NULLIF(excluded.executable_path,''),processes.executable_path),command_line=COALESCE(NULLIF(excluded.command_line,''),processes.command_line),ended_at_ns=NULL`, sample.ProcessID, sample.PID, sample.ParentPID, sample.ImageName, nullableString(sample.ExecutablePath), nullableString(sample.CommandLine), sample.StartedAtNS, snapshot.SampledAtNS); err != nil {
			return err
		}
		var cpuTimeMS any
		if sample.CPUTimeNS != nil && *sample.CPUTimeNS >= 0 {
			cpuTimeMS = float64(*sample.CPUTimeNS) / 1e6
		}
		var workingSet any
		if sample.WorkingSetBytes != nil && *sample.WorkingSetBytes <= uint64(^uint64(0)>>1) {
			workingSet = int64(*sample.WorkingSetBytes)
		}
		quality := "complete"
		if cpuTimeMS == nil || workingSet == nil {
			quality = "partial"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO process_samples(process_id,sample_time_ns,cpu_time_ms,working_set_bytes,sample_quality) VALUES(?,?,?,?,?)`, sample.ProcessID, snapshot.SampledAtNS, cpuTimeMS, workingSet, quality); err != nil {
			return err
		}
	}
	for _, exited := range changes.Exited {
		var code any
		if exited.ExitCode != nil {
			code = *exited.ExitCode
		}
		if _, err := tx.ExecContext(ctx, `UPDATE processes SET ended_at_ns=?,exit_code=? WHERE process_id=? AND ended_at_ns IS NULL`, exited.EndedAtNS, code, exited.Process.ProcessID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) CloseActiveProcesses(ctx context.Context, endedAtNS int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE processes SET ended_at_ns=? WHERE ended_at_ns IS NULL`, endedAtNS)
	return err
}

func (s *SQLite) PruneProcessHistory(ctx context.Context, cutoffNS int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM process_samples WHERE process_id IN (SELECT process_id FROM processes WHERE ended_at_ns IS NOT NULL AND ended_at_ns < ?)`, cutoffNS); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM processes WHERE ended_at_ns IS NOT NULL AND ended_at_ns < ?`, cutoffNS)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *SQLite) ListProcesses(ctx context.Context, limit int, beforeAtNS *int64, beforeID string) ([]process.Record, error) {
	var cursorTime any
	if beforeAtNS != nil {
		cursorTime = *beforeAtNS
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT p.process_id,p.pid,p.parent_pid,p.image_name,p.executable_path,p.command_line,p.started_at_ns,p.ended_at_ns,p.exit_code,
       latest.sample_time_ns,latest.cpu_time_ms,latest.working_set_bytes,latest.sample_quality,
       COALESCE(p.ended_at_ns,latest.sample_time_ns,p.started_at_ns) AS order_at_ns
FROM processes p
LEFT JOIN process_samples latest ON latest.process_id=p.process_id
  AND latest.sample_time_ns=(SELECT MAX(sample_time_ns) FROM process_samples WHERE process_id=p.process_id)
WHERE (? IS NULL OR COALESCE(p.ended_at_ns,latest.sample_time_ns,p.started_at_ns) < ?
       OR (COALESCE(p.ended_at_ns,latest.sample_time_ns,p.started_at_ns) = ? AND p.process_id < ?))
ORDER BY order_at_ns DESC,p.process_id DESC
LIMIT ?`, cursorTime, cursorTime, cursorTime, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]process.Record, 0, limit)
	for rows.Next() {
		var item process.Record
		var executablePath, commandLine sql.NullString
		var ended, lastSample sql.NullInt64
		var exitCode, workingSet sql.NullInt64
		var cpu sql.NullFloat64
		var quality sql.NullString
		if err := rows.Scan(&item.ProcessID, &item.PID, &item.ParentPID, &item.ImageName, &executablePath, &commandLine, &item.StartedAtNS, &ended, &exitCode, &lastSample, &cpu, &workingSet, &quality, &item.OrderAtNS); err != nil {
			return nil, err
		}
		item.ExecutablePath, item.CommandLine = executablePath.String, commandLine.String
		item.EndedAtNS = nullableInt64(ended)
		item.LastSampleAtNS = nullableInt64(lastSample)
		if exitCode.Valid && exitCode.Int64 >= 0 {
			value := uint32(exitCode.Int64)
			item.ExitCode = &value
		}
		if cpu.Valid {
			value := cpu.Float64
			item.CPUTimeMS = &value
		}
		if workingSet.Valid && workingSet.Int64 >= 0 {
			value := uint64(workingSet.Int64)
			item.WorkingSetBytes = &value
		}
		item.SampleQuality = quality.String
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) ListProcessesForSession(ctx context.Context, sessionID string) ([]process.Record, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.process_id,p.pid,p.parent_pid,p.image_name,p.executable_path,p.command_line,p.started_at_ns,p.ended_at_ns,p.exit_code,
       latest.sample_time_ns,latest.cpu_time_ms,latest.working_set_bytes,latest.sample_quality,
       COALESCE(p.ended_at_ns,latest.sample_time_ns,p.started_at_ns) AS order_at_ns
FROM processes p
LEFT JOIN process_samples latest ON latest.process_id=p.process_id
  AND latest.sample_time_ns=(SELECT MAX(sample_time_ns) FROM process_samples WHERE process_id=p.process_id)
WHERE EXISTS (
  SELECT 1 FROM spans s
  WHERE s.session_id=? AND s.span_role='tool' AND s.start_time_ns>0 AND s.end_time_ns>s.start_time_ns
    AND p.started_at_ns<=s.end_time_ns AND (p.ended_at_ns IS NULL OR p.ended_at_ns>=s.start_time_ns)
)
ORDER BY p.started_at_ns,p.process_id
LIMIT 500`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]process.Record, 0)
	for rows.Next() {
		var item process.Record
		var executablePath, commandLine sql.NullString
		var ended, lastSample sql.NullInt64
		var exitCode, workingSet sql.NullInt64
		var cpu sql.NullFloat64
		var quality sql.NullString
		if err := rows.Scan(&item.ProcessID, &item.PID, &item.ParentPID, &item.ImageName, &executablePath, &commandLine, &item.StartedAtNS, &ended, &exitCode, &lastSample, &cpu, &workingSet, &quality, &item.OrderAtNS); err != nil {
			return nil, err
		}
		item.ExecutablePath, item.CommandLine = executablePath.String, commandLine.String
		item.EndedAtNS = nullableInt64(ended)
		item.LastSampleAtNS = nullableInt64(lastSample)
		if exitCode.Valid && exitCode.Int64 >= 0 {
			value := uint32(exitCode.Int64)
			item.ExitCode = &value
		}
		if cpu.Valid {
			value := cpu.Float64
			item.CPUTimeMS = &value
		}
		if workingSet.Valid && workingSet.Int64 >= 0 {
			value := uint64(workingSet.Int64)
			item.WorkingSetBytes = &value
		}
		item.SampleQuality = quality.String
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) ListProcessToolSpans(ctx context.Context, fromNS, toNS int64) ([]correlation.ToolSpan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(s.session_id,''),s.span_id,tc.tool_name,tc.external_tool_call_id,
       s.start_time_ns,s.end_time_ns,s.process_pid,s.parent_process_pid
FROM spans s
JOIN tool_calls tc ON tc.trace_id=s.trace_id AND tc.span_id=s.span_id
WHERE s.span_role='tool' AND s.start_time_ns <= ? AND s.end_time_ns >= ? AND s.end_time_ns > s.start_time_ns
ORDER BY s.start_time_ns,s.span_id`, toNS, fromNS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spans := make([]correlation.ToolSpan, 0)
	for rows.Next() {
		var span correlation.ToolSpan
		var processPID, parentPID sql.NullInt64
		var toolCallID sql.NullString
		if err := rows.Scan(&span.SessionID, &span.SpanID, &span.ToolName, &toolCallID, &span.StartTimeNS, &span.EndTimeNS, &processPID, &parentPID); err != nil {
			return nil, err
		}
		if toolCallID.Valid {
			span.ToolCallID = toolCallID.String
		}
		if processPID.Valid && processPID.Int64 > 0 && processPID.Int64 <= int64(^uint32(0)) {
			value := uint32(processPID.Int64)
			span.ProcessPID = &value
		}
		if parentPID.Valid && parentPID.Int64 > 0 && parentPID.Int64 <= int64(^uint32(0)) {
			value := uint32(parentPID.Int64)
			span.ParentProcessPID = &value
		}
		spans = append(spans, span)
	}
	return spans, rows.Err()
}

func (s *SQLite) ListProcessToolSpansForSession(ctx context.Context, sessionID string) ([]correlation.ToolSpan, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(s.session_id,''),s.span_id,tc.tool_name,tc.external_tool_call_id,
       s.start_time_ns,s.end_time_ns,s.process_pid,s.parent_process_pid
FROM spans s
JOIN tool_calls tc ON tc.trace_id=s.trace_id AND tc.span_id=s.span_id
WHERE s.session_id=? AND s.span_role='tool' AND s.start_time_ns>0 AND s.end_time_ns>s.start_time_ns
ORDER BY s.start_time_ns,s.span_id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spans := make([]correlation.ToolSpan, 0)
	for rows.Next() {
		var span correlation.ToolSpan
		var processPID, parentPID sql.NullInt64
		var toolCallID sql.NullString
		if err := rows.Scan(&span.SessionID, &span.SpanID, &span.ToolName, &toolCallID, &span.StartTimeNS, &span.EndTimeNS, &processPID, &parentPID); err != nil {
			return nil, err
		}
		if toolCallID.Valid {
			span.ToolCallID = toolCallID.String
		}
		if processPID.Valid && processPID.Int64 > 0 && processPID.Int64 <= int64(^uint32(0)) {
			value := uint32(processPID.Int64)
			span.ProcessPID = &value
		}
		if parentPID.Valid && parentPID.Int64 > 0 && parentPID.Int64 <= int64(^uint32(0)) {
			value := uint32(parentPID.Int64)
			span.ParentProcessPID = &value
		}
		spans = append(spans, span)
	}
	return spans, rows.Err()
}
