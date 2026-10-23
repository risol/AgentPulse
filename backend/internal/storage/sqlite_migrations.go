package storage

import (
	"context"
	"database/sql"
	"fmt"
)

func Initialize(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than this application supports (%d)", version, schemaVersion)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	if version < 1 {
		if err := migrateToV1(ctx, db); err != nil {
			return err
		}
		version = 1
	}
	if version < 2 {
		if err := migrateToV2(ctx, db); err != nil {
			return err
		}
		version = 2
	}
	if version < 3 {
		if err := migrateToV3(ctx, db); err != nil {
			return err
		}
		version = 3
	}
	if version < 4 {
		if err := migrateToV4(ctx, db); err != nil {
			return err
		}
		version = 4
	}
	if version < 5 {
		if err := migrateToV5(ctx, db); err != nil {
			return err
		}
		version = 5
	}
	if version < 6 {
		if err := migrateToV6(ctx, db); err != nil {
			return err
		}
		version = 6
	}
	if version < 7 {
		if err := migrateToV7(ctx, db); err != nil {
			return err
		}
		version = 7
	}
	if version < 8 {
		if err := migrateToV8(ctx, db); err != nil {
			return err
		}
		version = 8
	}
	if version < 9 {
		if err := migrateToV9(ctx, db); err != nil {
			return err
		}
		version = 9
	}
	if version < 10 {
		if err := migrateToV10(ctx, db); err != nil {
			return err
		}
		version = 10
	}
	if version < 11 {
		if err := migrateToV11(ctx, db); err != nil {
			return err
		}
		version = 11
	}
	if version < 12 {
		if err := migrateToV12(ctx, db); err != nil {
			return err
		}
		version = 12
	}
	if version < 13 {
		if err := migrateToV13(ctx, db); err != nil {
			return err
		}
		version = 13
	}
	if version < 14 {
		return migrateToV14(ctx, db)
	}
	return nil
}

func migrateToV11(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE INDEX telemetry_events_trace_span_idx ON telemetry_events(source_id,trace_id,span_id)
  WHERE trace_id IS NOT NULL AND span_id IS NOT NULL;
PRAGMA user_version=11;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV12(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE INDEX telemetry_events_signal_received_idx ON telemetry_events(signal_type,received_at_ns);
CREATE INDEX telemetry_events_signal_trace_span_idx ON telemetry_events(signal_type,trace_id,span_id,id)
  WHERE trace_id IS NOT NULL AND span_id IS NOT NULL;
PRAGMA user_version=12;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV13(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE processes ADD COLUMN executable_path TEXT;
ALTER TABLE processes ADD COLUMN command_line TEXT;
PRAGMA user_version=13;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV14(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE metric_points ADD COLUMN start_time_ns INTEGER;
ALTER TABLE metric_points ADD COLUMN temporality TEXT NOT NULL DEFAULT '';
ALTER TABLE metric_points ADD COLUMN monotonic INTEGER;
ALTER TABLE metric_points ADD COLUMN aggregation_json TEXT NOT NULL DEFAULT '{}';
PRAGMA user_version=14;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV5(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE INDEX tool_calls_start_tool_idx ON tool_calls(start_time_ns,tool_name);
PRAGMA user_version=5;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV6(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The backfill joins every span against telemetry events. Without this
	// temporary index, a legacy database makes that update scan all events once
	// per span.
	if _, err := tx.ExecContext(ctx, `CREATE INDEX telemetry_events_received_span_migration_idx ON telemetry_events(signal_type,trace_id,span_id,received_at_ns);`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE spans ADD COLUMN received_at_ns INTEGER NOT NULL DEFAULT 0;
UPDATE spans SET received_at_ns=COALESCE(
  (SELECT MAX(e.received_at_ns) FROM telemetry_events e
   WHERE e.signal_type='traces' AND e.trace_id=spans.trace_id AND e.span_id=spans.span_id),
  NULLIF(start_time_ns,0),0);
DROP INDEX telemetry_events_received_span_migration_idx;
CREATE INDEX spans_received_idx ON spans(received_at_ns DESC);
PRAGMA user_version=6;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV7(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE ingest_diagnostics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  signal_type TEXT NOT NULL CHECK (signal_type IN ('logs','metrics','traces')),
  error_code TEXT NOT NULL CHECK (error_code IN ('unsupported_content_type','request_too_large','invalid_request_body','invalid_protobuf','storage_failure')),
  occurred_at_ns INTEGER NOT NULL
);
CREATE INDEX ingest_diagnostics_time_idx ON ingest_diagnostics(occurred_at_ns DESC,id DESC);
PRAGMA user_version=7;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV8(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE spans ADD COLUMN span_role TEXT NOT NULL DEFAULT 'other' CHECK (span_role IN ('model','tool','other'));
ALTER TABLE spans ADD COLUMN model TEXT;
UPDATE spans SET
  span_role=CASE
    WHEN COALESCE(json_extract(attributes_json,'$."tool.name"'),json_extract(attributes_json,'$."tool_name"')) IS NOT NULL THEN 'tool'
    WHEN COALESCE(json_extract(attributes_json,'$."gen_ai.operation.name"'),json_extract(attributes_json,'$."model"'),json_extract(attributes_json,'$."gen_ai.response.model"'),json_extract(attributes_json,'$."gen_ai.request.model"')) IS NOT NULL THEN 'model'
    ELSE 'other'
  END,
  model=NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."model"'),json_extract(attributes_json,'$."gen_ai.response.model"'),json_extract(attributes_json,'$."gen_ai.request.model"')) AS TEXT),'');
CREATE INDEX spans_session_role_start_idx ON spans(session_id,span_role,start_time_ns);
PRAGMA user_version=8;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV9(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE telemetry_events ADD COLUMN ttft_ms REAL;
ALTER TABLE spans ADD COLUMN process_pid INTEGER;
ALTER TABLE spans ADD COLUMN parent_process_pid INTEGER;
UPDATE telemetry_events SET ttft_ms=CASE
  WHEN json_type(attributes_json,'$."ttft_ms"') IN ('integer','real') AND CAST(json_extract(attributes_json,'$."ttft_ms"') AS REAL)>=0
  THEN CAST(json_extract(attributes_json,'$."ttft_ms"') AS REAL) END;
UPDATE spans SET
  process_pid=CASE WHEN json_type(attributes_json,'$."process.pid"')='integer' THEN CAST(json_extract(attributes_json,'$."process.pid"') AS INTEGER) END,
  parent_process_pid=CASE WHEN json_type(attributes_json,'$."process.parent_pid"')='integer' THEN CAST(json_extract(attributes_json,'$."process.parent_pid"') AS INTEGER) END;
PRAGMA user_version=9;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV10(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE telemetry_events ADD COLUMN input_tokens INTEGER;
ALTER TABLE telemetry_events ADD COLUMN output_tokens INTEGER;
ALTER TABLE telemetry_events ADD COLUMN reasoning_tokens INTEGER;
UPDATE telemetry_events SET
  input_tokens=CASE WHEN json_type(attributes_json,'$."input_token_count"')='integer' AND CAST(json_extract(attributes_json,'$."input_token_count"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."input_token_count"') AS INTEGER)
                    WHEN json_type(attributes_json,'$."gen_ai.usage.input_tokens"')='integer' AND CAST(json_extract(attributes_json,'$."gen_ai.usage.input_tokens"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."gen_ai.usage.input_tokens"') AS INTEGER) END,
  output_tokens=CASE WHEN json_type(attributes_json,'$."output_token_count"')='integer' AND CAST(json_extract(attributes_json,'$."output_token_count"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."output_token_count"') AS INTEGER)
                     WHEN json_type(attributes_json,'$."gen_ai.usage.output_tokens"')='integer' AND CAST(json_extract(attributes_json,'$."gen_ai.usage.output_tokens"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."gen_ai.usage.output_tokens"') AS INTEGER) END,
  reasoning_tokens=CASE WHEN json_type(attributes_json,'$."reasoning_token_count"')='integer' AND CAST(json_extract(attributes_json,'$."reasoning_token_count"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."reasoning_token_count"') AS INTEGER)
                       WHEN json_type(attributes_json,'$."codex.usage.reasoning_output_tokens"')='integer' AND CAST(json_extract(attributes_json,'$."codex.usage.reasoning_output_tokens"') AS INTEGER)>=0 THEN CAST(json_extract(attributes_json,'$."codex.usage.reasoning_output_tokens"') AS INTEGER) END
WHERE signal_type='logs';
PRAGMA user_version=10;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV1(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS telemetry_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  signal_type TEXT NOT NULL CHECK (signal_type IN ('logs', 'metrics', 'traces')),
  event_time_ns INTEGER,
  received_at_ns INTEGER NOT NULL,
  trace_id TEXT,
  span_id TEXT,
  event_name TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS telemetry_events_received_idx ON telemetry_events(received_at_ns DESC);
CREATE INDEX IF NOT EXISTS telemetry_events_signal_time_idx ON telemetry_events(signal_type, event_time_ns);
CREATE TABLE IF NOT EXISTS spans (
  trace_id TEXT NOT NULL,
  span_id TEXT NOT NULL,
  parent_span_id TEXT,
  name TEXT NOT NULL,
  start_time_ns INTEGER NOT NULL,
  end_time_ns INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY(trace_id, span_id)
);
CREATE INDEX IF NOT EXISTS spans_start_idx ON spans(start_time_ns);
CREATE TABLE IF NOT EXISTS metric_points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  time_ns INTEGER,
  numeric_value REAL,
  unit TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS metric_points_name_time_idx ON metric_points(metric_name, time_ns);
CREATE TABLE IF NOT EXISTS app_state (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at_ns INTEGER NOT NULL
);
PRAGMA user_version=1;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV2(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE metric_points ADD COLUMN received_at_ns INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS metric_points_received_idx ON metric_points(received_at_ns DESC);
UPDATE metric_points SET received_at_ns=COALESCE(time_ns, 0) WHERE received_at_ns=0;
PRAGMA user_version=2;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV3(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE processes (
  process_id TEXT PRIMARY KEY,
  pid INTEGER NOT NULL,
  parent_pid INTEGER NOT NULL,
  image_name TEXT NOT NULL,
  started_at_ns INTEGER NOT NULL,
  ended_at_ns INTEGER,
  exit_code INTEGER,
  association_method TEXT NOT NULL DEFAULT 'unassigned',
  association_confidence REAL,
  created_at_ns INTEGER NOT NULL
);
CREATE INDEX processes_pid_started_idx ON processes(pid, started_at_ns DESC);
CREATE INDEX processes_end_idx ON processes(ended_at_ns DESC);
CREATE TABLE process_samples (
  process_id TEXT NOT NULL REFERENCES processes(process_id) ON DELETE CASCADE,
  sample_time_ns INTEGER NOT NULL,
  cpu_time_ms REAL,
  working_set_bytes INTEGER,
  sample_quality TEXT NOT NULL CHECK (sample_quality IN ('complete', 'partial')),
  PRIMARY KEY(process_id, sample_time_ns)
);
CREATE INDEX process_samples_time_idx ON process_samples(sample_time_ns DESC);
PRAGMA user_version=3;
`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateToV4(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
ALTER TABLE telemetry_events ADD COLUMN source_id TEXT NOT NULL DEFAULT 'codex';
ALTER TABLE telemetry_events ADD COLUMN external_session_id TEXT;
ALTER TABLE telemetry_events ADD COLUMN session_id TEXT;
ALTER TABLE telemetry_events ADD COLUMN external_turn_id TEXT;
ALTER TABLE telemetry_events ADD COLUMN turn_id TEXT;
ALTER TABLE telemetry_events ADD COLUMN tool_call_id TEXT;
ALTER TABLE telemetry_events ADD COLUMN correlation_method TEXT NOT NULL DEFAULT 'unassigned';
ALTER TABLE telemetry_events ADD COLUMN data_quality TEXT NOT NULL DEFAULT 'partial';
ALTER TABLE spans ADD COLUMN source_id TEXT NOT NULL DEFAULT 'codex';
ALTER TABLE spans ADD COLUMN external_session_id TEXT;
ALTER TABLE spans ADD COLUMN session_id TEXT;
ALTER TABLE spans ADD COLUMN external_turn_id TEXT;
ALTER TABLE spans ADD COLUMN turn_id TEXT;
ALTER TABLE spans ADD COLUMN correlation_method TEXT NOT NULL DEFAULT 'unassigned';
ALTER TABLE spans ADD COLUMN correlation_confidence REAL;
CREATE TABLE sessions (
  session_id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  external_id TEXT NOT NULL,
  started_at_ns INTEGER,
  last_activity_ns INTEGER,
  model TEXT,
  event_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_tokens INTEGER,
  data_quality TEXT NOT NULL CHECK (data_quality IN ('complete', 'partial', 'inferred')),
  created_at_ns INTEGER NOT NULL,
  UNIQUE(source_id,external_id)
);
CREATE INDEX sessions_activity_idx ON sessions(last_activity_ns DESC,session_id DESC);
CREATE TABLE turns (
  turn_id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  external_id TEXT NOT NULL,
  session_id TEXT,
  started_at_ns INTEGER,
  last_activity_ns INTEGER,
  model TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_tokens INTEGER,
  data_quality TEXT NOT NULL CHECK (data_quality IN ('complete', 'partial', 'inferred')),
  FOREIGN KEY(session_id) REFERENCES sessions(session_id) ON DELETE SET NULL,
  UNIQUE(source_id,external_id)
);
CREATE INDEX turns_session_start_idx ON turns(session_id,started_at_ns);
CREATE TABLE tool_calls (
  tool_call_id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  external_tool_call_id TEXT,
  session_id TEXT,
  turn_id TEXT,
  tool_name TEXT NOT NULL,
  tool_type TEXT,
  start_time_ns INTEGER,
  end_time_ns INTEGER,
  duration_ms REAL,
  status TEXT,
  retry_index INTEGER,
  input_summary TEXT,
  output_summary TEXT,
  trace_id TEXT,
  span_id TEXT,
  source_event_id INTEGER,
  correlation_method TEXT NOT NULL,
  correlation_confidence REAL,
  data_quality TEXT NOT NULL CHECK (data_quality IN ('complete', 'partial', 'inferred')),
  FOREIGN KEY(session_id) REFERENCES sessions(session_id) ON DELETE SET NULL,
  FOREIGN KEY(turn_id) REFERENCES turns(turn_id) ON DELETE SET NULL,
  FOREIGN KEY(source_event_id) REFERENCES telemetry_events(id) ON DELETE SET NULL
);
CREATE INDEX tool_calls_session_start_idx ON tool_calls(session_id,start_time_ns DESC);
CREATE INDEX tool_calls_external_idx ON tool_calls(source_id,external_tool_call_id);
CREATE INDEX telemetry_events_session_time_idx ON telemetry_events(session_id,event_time_ns);
CREATE INDEX telemetry_events_unassigned_idx ON telemetry_events(id DESC) WHERE session_id IS NULL;
CREATE INDEX spans_session_start_idx ON spans(session_id,start_time_ns);
`); err != nil {
		return err
	}
	if err := backfillCanonicalTelemetry(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version=4;`); err != nil {
		return err
	}
	return tx.Commit()
}

func backfillCanonicalTelemetry(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
UPDATE telemetry_events SET
  external_session_id=NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT),''),
  external_turn_id=NULLIF(CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT),''),
  tool_call_id=NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."tool.call_id"'),json_extract(attributes_json,'$."call_id"')) AS TEXT),''),
  session_id=CASE WHEN NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT),'') IS NULL THEN NULL
                  ELSE source_id||':'||CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT) END,
  turn_id=CASE WHEN NULLIF(CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT),'') IS NULL THEN NULL
               ELSE source_id||':'||CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT) END,
  correlation_method=CASE WHEN COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) IS NOT NULL THEN 'observed_identifier'
                          WHEN json_extract(attributes_json,'$."turn.id"') IS NOT NULL THEN 'observed_turn_identifier' ELSE 'unassigned' END,
  data_quality=CASE WHEN COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) IS NOT NULL AND event_time_ns IS NOT NULL THEN 'complete' ELSE 'partial' END;
UPDATE spans SET
  external_session_id=NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT),''),
  external_turn_id=NULLIF(CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT),''),
  session_id=CASE WHEN NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT),'') IS NULL THEN NULL
                  ELSE source_id||':'||CAST(COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) AS TEXT) END,
  turn_id=CASE WHEN NULLIF(CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT),'') IS NULL THEN NULL
               ELSE source_id||':'||CAST(json_extract(attributes_json,'$."turn.id"') AS TEXT) END,
  correlation_method=CASE WHEN COALESCE(json_extract(attributes_json,'$."conversation.id"'),json_extract(attributes_json,'$."session.id"')) IS NOT NULL THEN 'observed_identifier'
                          WHEN json_extract(attributes_json,'$."turn.id"') IS NOT NULL THEN 'observed_turn_identifier' ELSE 'unassigned' END;
INSERT INTO sessions(session_id,source_id,external_id,started_at_ns,last_activity_ns,model,event_count,input_tokens,output_tokens,reasoning_tokens,data_quality,created_at_ns)
SELECT session_id,source_id,external_session_id,
       MIN(COALESCE(event_time_ns,received_at_ns)),MAX(COALESCE(event_time_ns,received_at_ns)),
       MAX(COALESCE(json_extract(attributes_json,'$."model"'),json_extract(attributes_json,'$."gen_ai.response.model"'),json_extract(attributes_json,'$."gen_ai.request.model"'))),
       COUNT(*),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."input_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."input_token_count"') AS INTEGER) END),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."output_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."output_token_count"') AS INTEGER) END),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."reasoning_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."reasoning_token_count"') AS INTEGER) END),
       CASE WHEN COUNT(event_time_ns)=COUNT(*) THEN 'complete' ELSE 'partial' END,MIN(received_at_ns)
FROM telemetry_events WHERE session_id IS NOT NULL
GROUP BY session_id,source_id,external_session_id;
INSERT INTO turns(turn_id,source_id,external_id,session_id,started_at_ns,last_activity_ns,model,input_tokens,output_tokens,reasoning_tokens,data_quality)
SELECT turn_id,source_id,external_turn_id,MAX(session_id),
       MIN(COALESCE(event_time_ns,received_at_ns)),MAX(COALESCE(event_time_ns,received_at_ns)),
       MAX(COALESCE(json_extract(attributes_json,'$."model"'),json_extract(attributes_json,'$."gen_ai.response.model"'),json_extract(attributes_json,'$."gen_ai.request.model"'))),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."input_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."input_token_count"') AS INTEGER) END),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."output_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."output_token_count"') AS INTEGER) END),
       SUM(CASE WHEN signal_type='logs' AND json_type(attributes_json,'$."reasoning_token_count"') IS NOT NULL THEN CAST(json_extract(attributes_json,'$."reasoning_token_count"') AS INTEGER) END),
       CASE WHEN COUNT(event_time_ns)=COUNT(*) THEN 'complete' ELSE 'partial' END
FROM telemetry_events WHERE turn_id IS NOT NULL
GROUP BY turn_id,source_id,external_turn_id;
INSERT INTO tool_calls(tool_call_id,source_id,external_tool_call_id,session_id,turn_id,tool_name,tool_type,start_time_ns,duration_ms,status,trace_id,span_id,source_event_id,correlation_method,correlation_confidence,data_quality)
SELECT CASE WHEN NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."tool.call_id"'),json_extract(attributes_json,'$."call_id"')) AS TEXT),'') IS NOT NULL
            THEN COALESCE(session_id,source_id)||':tool:'||CAST(COALESCE(json_extract(attributes_json,'$."tool.call_id"'),json_extract(attributes_json,'$."call_id"')) AS TEXT)
            WHEN trace_id IS NOT NULL AND span_id IS NOT NULL THEN source_id||':span:'||trace_id||':'||span_id
            ELSE source_id||':event:'||CAST(id AS TEXT) END,
       source_id,NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."tool.call_id"'),json_extract(attributes_json,'$."call_id"')) AS TEXT),''),
       session_id,turn_id,COALESCE(NULLIF(CAST(json_extract(attributes_json,'$."tool.name"') AS TEXT),''),NULLIF(CAST(json_extract(attributes_json,'$."tool_name"') AS TEXT),'')),
       NULLIF(CAST(json_extract(attributes_json,'$."tool.type"') AS TEXT),''),event_time_ns,
       CASE WHEN json_type(attributes_json,'$."duration_ms"') IN ('integer','real') THEN CAST(json_extract(attributes_json,'$."duration_ms"') AS REAL) END,
       CASE WHEN json_type(attributes_json,'$."success"')='true' THEN 'success' WHEN json_type(attributes_json,'$."success"')='false' THEN 'error'
            ELSE NULLIF(CAST(COALESCE(json_extract(attributes_json,'$."outcome"'),json_extract(attributes_json,'$."error.type"')) AS TEXT),'') END,
       trace_id,span_id,id,correlation_method,
       CASE WHEN session_id IS NOT NULL OR turn_id IS NOT NULL THEN 1.0 END,
       CASE WHEN event_time_ns IS NOT NULL AND (trace_id IS NOT NULL OR json_extract(attributes_json,'$."tool.call_id"') IS NOT NULL OR json_extract(attributes_json,'$."call_id"') IS NOT NULL) THEN 'complete' ELSE 'partial' END
FROM telemetry_events
WHERE COALESCE(json_extract(attributes_json,'$."tool.name"'),json_extract(attributes_json,'$."tool_name"')) IS NOT NULL
ON CONFLICT(tool_call_id) DO UPDATE SET
  session_id=CASE WHEN tool_calls.session_id IS NOT NULL AND excluded.session_id IS NOT NULL AND tool_calls.session_id!=excluded.session_id THEN tool_calls.session_id ELSE COALESCE(excluded.session_id,tool_calls.session_id) END,
  turn_id=CASE WHEN tool_calls.turn_id IS NOT NULL AND excluded.turn_id IS NOT NULL AND tool_calls.turn_id!=excluded.turn_id THEN tool_calls.turn_id ELSE COALESCE(excluded.turn_id,tool_calls.turn_id) END,
  start_time_ns=CASE WHEN tool_calls.start_time_ns IS NULL THEN excluded.start_time_ns WHEN excluded.start_time_ns IS NULL THEN tool_calls.start_time_ns ELSE MIN(tool_calls.start_time_ns,excluded.start_time_ns) END,
  duration_ms=COALESCE(excluded.duration_ms,tool_calls.duration_ms),status=COALESCE(excluded.status,tool_calls.status),
  source_event_id=excluded.source_event_id,trace_id=COALESCE(excluded.trace_id,tool_calls.trace_id),span_id=COALESCE(excluded.span_id,tool_calls.span_id),
  correlation_method=CASE WHEN excluded.session_id IS NOT NULL OR excluded.turn_id IS NOT NULL THEN excluded.correlation_method ELSE tool_calls.correlation_method END,
  correlation_confidence=COALESCE(excluded.correlation_confidence,tool_calls.correlation_confidence),
  data_quality=CASE WHEN excluded.data_quality='complete' THEN 'complete' ELSE tool_calls.data_quality END;
`); err != nil {
		return err
	}
	return nil
}
