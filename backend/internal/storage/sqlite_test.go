package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/codexadapter"
	"github.com/agentpulse/agentpulse/backend/internal/process"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	_ "modernc.org/sqlite"
)

func TestConfigureConnectionPoolAllowsConcurrentQueries(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ConfigureConnectionPool(db)
	if got := db.Stats().MaxOpenConnections; got != 8 {
		t.Fatalf("max open connections = %d, want 8", got)
	}
}

func TestInitializeMigratesVersionOneMetricReceiptTime(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
CREATE TABLE telemetry_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  signal_type TEXT NOT NULL,
  event_time_ns INTEGER,
  received_at_ns INTEGER NOT NULL,
  trace_id TEXT,
  span_id TEXT,
  event_name TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE spans (
  trace_id TEXT NOT NULL,
  span_id TEXT NOT NULL,
  parent_span_id TEXT,
  name TEXT NOT NULL,
  start_time_ns INTEGER NOT NULL,
  end_time_ns INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY(trace_id,span_id)
);
CREATE TABLE metric_points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  time_ns INTEGER,
  numeric_value REAL,
  unit TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO metric_points(metric_name,metric_type,time_ns,numeric_value) VALUES('codex.tool.call','sum',123,1);
PRAGMA user_version=1;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}
	var receivedAt int64
	if err := db.QueryRow("SELECT received_at_ns FROM metric_points WHERE metric_name='codex.tool.call'").Scan(&receivedAt); err != nil {
		t.Fatal(err)
	}
	if receivedAt != 123 {
		t.Fatalf("migrated receipt time = %d, want existing metric time 123", receivedAt)
	}
}

func TestInitializeMigratesVersionTenTraceSpanEventIndex(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX telemetry_events_trace_span_idx;
DROP INDEX telemetry_events_signal_received_idx;
DROP INDEX telemetry_events_signal_trace_span_idx;
ALTER TABLE processes DROP COLUMN command_line;
ALTER TABLE processes DROP COLUMN executable_path;
ALTER TABLE metric_points DROP COLUMN aggregation_json;
ALTER TABLE metric_points DROP COLUMN monotonic;
ALTER TABLE metric_points DROP COLUMN temporality;
ALTER TABLE metric_points DROP COLUMN start_time_ns;
PRAGMA user_version=10;
INSERT INTO telemetry_events(signal_type,received_at_ns,trace_id,span_id,event_name,attributes_json)
VALUES('logs',100,'trace-1','span-1','event','{}');`); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var indexCount, eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='telemetry_events_trace_span_idx'`).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM telemetry_events WHERE trace_id='trace-1' AND span_id='span-1'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 || eventCount != 1 {
		t.Fatalf("v10 migration left index/event counts %d/%d, want 1/1", indexCount, eventCount)
	}
}

func TestInitializeMigratesVersionTwelveProcessPrivacyColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
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
CREATE TABLE metric_points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  time_ns INTEGER,
  numeric_value REAL,
  unit TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  received_at_ns INTEGER NOT NULL DEFAULT 0
);
INSERT INTO metric_points(metric_name,metric_type,time_ns,numeric_value)
VALUES('legacy.duration','histogram',100,2);
INSERT INTO processes(process_id,pid,parent_pid,image_name,started_at_ns,created_at_ns)
VALUES('process-1',1,0,'codex.exe',100,100);
PRAGMA user_version=12;`); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	var executablePath, commandLine sql.NullString
	if err := db.QueryRow(`SELECT executable_path,command_line FROM processes WHERE process_id='process-1'`).Scan(&executablePath, &commandLine); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion || executablePath.Valid || commandLine.Valid {
		t.Fatalf("v12 migration version=%d path=%+v command=%+v; want latest schema and NULL legacy details", version, executablePath, commandLine)
	}
	var metricType, aggregation string
	if err := db.QueryRow(`SELECT metric_type,aggregation_json FROM metric_points WHERE metric_name='legacy.duration'`).Scan(&metricType, &aggregation); err != nil {
		t.Fatal(err)
	}
	if metricType != "histogram" || aggregation != "{}" {
		t.Fatalf("migration changed legacy metric data: type=%q aggregation=%q", metricType, aggregation)
	}
}

func TestInitializeMigratesVersionThirteenMetricAggregationFields(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE metric_points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  time_ns INTEGER,
  numeric_value REAL,
  unit TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  received_at_ns INTEGER NOT NULL DEFAULT 0
);
INSERT INTO metric_points(metric_name,metric_type,time_ns,numeric_value,received_at_ns)
VALUES('legacy.sum','sum',200,3,250);
PRAGMA user_version=13;`); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var version int
	var metricType, aggregation, temporality string
	var value float64
	var monotonic sql.NullBool
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT metric_type,numeric_value,temporality,monotonic,aggregation_json FROM metric_points WHERE metric_name='legacy.sum'`).Scan(&metricType, &value, &temporality, &monotonic, &aggregation); err != nil {
		t.Fatal(err)
	}
	if version != 14 || metricType != "sum" || value != 3 || temporality != "" || monotonic.Valid || aggregation != "{}" {
		t.Fatalf("v13 migration version=%d metric=%q value=%g temporality=%q monotonic=%+v aggregation=%q", version, metricType, value, temporality, monotonic, aggregation)
	}
}

func TestInitializeMigratesVersionSevenCanonicalSpanMetadata(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE spans (
  trace_id TEXT NOT NULL,
  span_id TEXT NOT NULL,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  session_id TEXT,
  start_time_ns INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(trace_id,span_id)
);
CREATE TABLE metric_points (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  time_ns INTEGER,
  numeric_value REAL,
  unit TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
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
CREATE TABLE telemetry_events (id INTEGER PRIMARY KEY,signal_type TEXT NOT NULL DEFAULT 'logs',event_time_ns INTEGER,received_at_ns INTEGER NOT NULL DEFAULT 0,trace_id TEXT,span_id TEXT,attributes_json TEXT NOT NULL DEFAULT '{}',source_id TEXT NOT NULL DEFAULT 'codex');
INSERT INTO spans(trace_id,span_id,attributes_json,session_id,start_time_ns)
VALUES('trace','model','{"gen_ai.response.model":"gpt-test"}','codex:session',10),
      ('trace','tool','{"tool.name":"shell","process.pid":42,"process.parent_pid":12}','codex:session',20);
INSERT INTO telemetry_events(id,signal_type,event_time_ns,attributes_json) VALUES(1,'logs',30,'{"ttft_ms":31.5,"input_token_count":19}');
PRAGMA user_version=7;`); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("migrated schema version = %d, want %d", version, schemaVersion)
	}
	var role, model string
	if err := db.QueryRow(`SELECT span_role,model FROM spans WHERE span_id='model'`).Scan(&role, &model); err != nil {
		t.Fatal(err)
	}
	if role != "model" || model != "gpt-test" {
		t.Fatalf("model span migrated to role=%q model=%q", role, model)
	}
	if err := db.QueryRow(`SELECT span_role FROM spans WHERE span_id='tool'`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "tool" {
		t.Fatalf("tool span migrated to role=%q, want tool", role)
	}
	var processPID, parentPID int64
	if err := db.QueryRow(`SELECT process_pid,parent_process_pid FROM spans WHERE span_id='tool'`).Scan(&processPID, &parentPID); err != nil {
		t.Fatal(err)
	}
	if processPID != 42 || parentPID != 12 {
		t.Fatalf("migrated process identifiers = %d/%d, want 42/12", processPID, parentPID)
	}
	var ttft float64
	if err := db.QueryRow(`SELECT ttft_ms FROM telemetry_events WHERE id=1`).Scan(&ttft); err != nil {
		t.Fatal(err)
	}
	if ttft != 31.5 {
		t.Fatalf("migrated TTFT = %v, want 31.5", ttft)
	}
	var inputTokens int64
	if err := db.QueryRow(`SELECT input_tokens FROM telemetry_events WHERE id=1`).Scan(&inputTokens); err != nil {
		t.Fatal(err)
	}
	if inputTokens != 19 {
		t.Fatalf("migrated input tokens = %d, want 19", inputTokens)
	}
}

func TestProcessSettingsAndSnapshotPersistence(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)

	config, err := store.LoadProcessMonitorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.CaptureExecutablePaths || config.CaptureCommandLines || len(config.RootProcessNames) != 1 || config.SampleIntervalMS != process.DefaultSampleIntervalMS {
		t.Fatalf("unexpected default process configuration: %+v", config)
	}
	config.Enabled = true
	config.RootProcessNames = []string{"Codex"}
	config.CaptureExecutablePaths = true
	config.CaptureCommandLines = true
	if err := store.SaveProcessMonitorConfig(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadProcessMonitorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Enabled || !loaded.CaptureExecutablePaths || !loaded.CaptureCommandLines || loaded.RootProcessNames[0] != "codex.exe" {
		t.Fatalf("configuration was not normalized and persisted: %+v", loaded)
	}

	tracker := process.NewTracker()
	cpuTime := int64(125_000_000)
	workingSet := uint64(8 * 1024 * 1024)
	sample := process.Sample{
		ProcessID: process.InstanceID(42, 500), PID: 42, ParentPID: 1, ImageName: "codex.exe",
		ExecutablePath: "%USERPROFILE%\\tools\\codex.exe", CommandLine: "codex exec --api-key=[redacted]",
		StartedAtNS: 500, SampledAtNS: 1000, CPUTimeNS: &cpuTime, WorkingSetBytes: &workingSet,
	}
	snapshot := process.Snapshot{SampledAtNS: 1000, Processes: []process.Sample{sample}}
	if err := store.ApplyProcessSnapshot(context.Background(), snapshot, tracker.Apply(snapshot)); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListProcesses(context.Background(), 10, nil, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("process list = %+v, error = %v", records, err)
	}
	if records[0].ProcessID != sample.ProcessID || records[0].CPUTimeMS == nil || *records[0].CPUTimeMS != 125 || records[0].WorkingSetBytes == nil || *records[0].WorkingSetBytes != workingSet || records[0].SampleQuality != "complete" || records[0].ExecutablePath != sample.ExecutablePath || records[0].CommandLine != sample.CommandLine {
		t.Fatalf("unexpected stored process sample: %+v", records[0])
	}

	ended := process.Snapshot{SampledAtNS: 2000}
	if err := store.ApplyProcessSnapshot(context.Background(), ended, tracker.Apply(ended)); err != nil {
		t.Fatal(err)
	}
	records, err = store.ListProcesses(context.Background(), 10, nil, "")
	if err != nil || len(records) != 1 || records[0].EndedAtNS == nil || *records[0].EndedAtNS != 2000 {
		t.Fatalf("process exit was not persisted: %+v, error = %v", records, err)
	}

	for index, startedAt := range []int64{3000, 4000} {
		pid := uint32(43 + index)
		item := process.Sample{ProcessID: process.InstanceID(pid, startedAt), PID: pid, ImageName: "runner.exe", StartedAtNS: startedAt, SampledAtNS: startedAt + 500}
		snapshot := process.Snapshot{SampledAtNS: item.SampledAtNS, Processes: []process.Sample{item}}
		if err := store.ApplyProcessSnapshot(context.Background(), snapshot, process.ChangeSet{}); err != nil {
			t.Fatal(err)
		}
	}
	firstPage, err := store.ListProcesses(context.Background(), 2, nil, "")
	if err != nil || len(firstPage) != 2 {
		t.Fatalf("first process page = %+v, error = %v", firstPage, err)
	}
	secondPage, err := store.ListProcesses(context.Background(), 2, &firstPage[1].OrderAtNS, firstPage[1].ProcessID)
	if err != nil || len(secondPage) != 1 || secondPage[0].ProcessID != sample.ProcessID {
		t.Fatalf("second process page = %+v, error = %v", secondPage, err)
	}
	deleted, err := store.PruneProcessHistory(context.Background(), 2001)
	if err != nil || deleted != 1 {
		t.Fatalf("pruned %d process records, error = %v; want 1", deleted, err)
	}
	remaining, err := store.ListProcesses(context.Background(), 10, nil, "")
	if err != nil || len(remaining) != 2 {
		t.Fatalf("process retention left %+v, error = %v; want two active records", remaining, err)
	}
	var staleSamples int
	if err := db.QueryRow(`SELECT COUNT(*) FROM process_samples WHERE process_id=?`, sample.ProcessID).Scan(&staleSamples); err != nil {
		t.Fatal(err)
	}
	if staleSamples != 0 {
		t.Fatalf("retention left %d samples for the pruned process", staleSamples)
	}
}

func TestTelemetryRetentionPrunesExpiredCanonicalHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)

	days, err := store.LoadTelemetryRetentionDays(context.Background())
	if err != nil || days != telemetry.DefaultTelemetryRetentionDays {
		t.Fatalf("default telemetry retention = %d days, error = %v", days, err)
	}
	if err := store.SaveTelemetryRetentionDays(context.Background(), 14); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTelemetryRetentionDays(context.Background(), 0); err == nil {
		t.Fatal("zero-day telemetry retention was accepted")
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour).UnixNano()
	old := cutoff - int64(time.Hour)
	recent := time.Now().UnixNano()
	if _, err := db.Exec(`
INSERT INTO sessions(session_id,source_id,external_id,started_at_ns,last_activity_ns,event_count,data_quality,created_at_ns)
VALUES('old-session','codex','old-session',?,?,1,'complete',?),('recent-session','codex','recent-session',?,?,1,'complete',?),('late-session','codex','late-session',?,?,1,'complete',?);
INSERT INTO turns(turn_id,source_id,external_id,session_id,started_at_ns,last_activity_ns,data_quality)
VALUES('old-turn','codex','old-turn','old-session',?,?,'complete');
INSERT INTO telemetry_events(signal_type,event_time_ns,received_at_ns,event_name,attributes_json,source_id,session_id,correlation_method,data_quality)
VALUES('logs',? ,?,'old_event','{}','codex','old-session','observed_identifier','complete'),
      ('logs',? ,?,'recent_event','{}','codex','recent-session','observed_identifier','complete'),
      ('logs',? ,?,'late_event','{}','codex','late-session','observed_identifier','complete');
INSERT INTO spans(trace_id,span_id,name,start_time_ns,end_time_ns,status_code,attributes_json,source_id,session_id,correlation_method,received_at_ns)
VALUES('old-trace','old-span','tool',?,?,0,'{}','codex','old-session','observed_identifier',?);
INSERT INTO metric_points(metric_name,metric_type,time_ns,numeric_value,received_at_ns)
VALUES('old.metric','sum',?,1,?);
INSERT INTO tool_calls(tool_call_id,source_id,session_id,turn_id,tool_name,start_time_ns,status,correlation_method,data_quality)
VALUES('old-tool','codex','old-session','old-turn','shell',?,'error','observed_identifier','complete');
`,
		old, old, old, recent, recent, recent, old, old, old, // session rows
		old, old, // turn row
		old, old, recent, recent, old, recent, // event rows
		old, old, old, // span row
		old, old, // metric row
		old, // tool call row
	); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.PruneTelemetryHistoryBatch(context.Background(), cutoff, 1)
	if err != nil {
		t.Fatalf("prune expired telemetry: %v", err)
	}
	if deleted < 6 {
		t.Fatalf("prune deleted %d rows, want at least six related rows", deleted)
	}
	for table, want := range map[string]int{"sessions": 2, "turns": 0, "tool_calls": 0, "spans": 0, "telemetry_events": 2, "metric_points": 0} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("%s row count = %d, want %d", table, count, want)
		}
	}
}

func TestIngestDiagnosticsAreAllowlistedAndBounded(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)
	for range 205 {
		if err := store.RecordIngestDiagnostic(context.Background(), telemetry.SignalLogs, "invalid_protobuf"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordIngestDiagnostic(context.Background(), telemetry.SignalLogs, `invalid_protobuf|raw payload`); err == nil {
		t.Fatal("unrecognized diagnostic code was accepted")
	}
	var persisted int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ingest_diagnostics`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	count, err := store.CountIngestDiagnosticsSince(context.Background(), 0)
	if err != nil || count != 200 || persisted != 200 {
		t.Fatalf("diagnostic rows/count = %d/%d, error=%v; want 200/200", persisted, count, err)
	}
	recent, err := store.ListRecentIngestDiagnostics(context.Background(), 10)
	if err != nil || len(recent) != 10 || recent[0].Code != "invalid_protobuf" || recent[0].Signal != telemetry.SignalLogs {
		t.Fatalf("recent diagnostics = %+v, error=%v", recent, err)
	}
}

func TestClearHistoryRemovesTelemetryAndProcessesButKeepsSettings(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)
	now := time.Now().UnixNano()
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		Signal: telemetry.SignalLogs, ReceivedAtNS: now,
		Events: []telemetry.Event{{Name: "log_record", EventTimeNS: &now, Attributes: map[string]any{"conversation.id": "clear-session", "tool_name": "shell"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTelemetryRetentionDays(context.Background(), 45); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.ClearHistory(context.Background())
	if err != nil || deleted == 0 {
		t.Fatalf("clear history deleted %d rows, error = %v", deleted, err)
	}
	for _, table := range []string{"sessions", "turns", "tool_calls", "spans", "telemetry_events", "metric_points", "processes", "process_samples"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s retained %d rows after clearing history", table, count)
		}
	}
	days, err := store.LoadTelemetryRetentionDays(context.Background())
	if err != nil || days != 45 {
		t.Fatalf("clearing history changed settings: retention=%d, error=%v", days, err)
	}
}

func TestTraceSpanRelationshipAssociatesEventsAcrossArrivalOrders(t *testing.T) {
	for _, eventFirst := range []bool{true, false} {
		name := "span-first"
		if eventFirst {
			name = "event-first"
		}
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if err := Initialize(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			store := NewSQLite(db)
			writer := codexadapter.NewWriter(store)
			eventAt := int64(100)
			events := telemetry.Batch{Signal: telemetry.SignalLogs, ReceivedAtNS: 110, Events: []telemetry.Event{{
				TraceID: "trace-1", SpanID: "model-span", EventTimeNS: &eventAt, Name: "completion",
				Attributes: map[string]any{"input_token_count": int64(7), "output_token_count": int64(3)},
			}}}
			spans := telemetry.Batch{Signal: telemetry.SignalTraces, ReceivedAtNS: 120, Spans: []telemetry.Span{{
				TraceID: "trace-1", SpanID: "model-span", StartTimeNS: 90, EndTimeNS: 105,
				Attributes: map[string]any{"conversation.id": "session-1", "turn.id": "turn-1", "gen_ai.response.model": "gpt-test"},
			}}}
			if eventFirst {
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
			}

			var sessionID, turnID, method string
			if err := db.QueryRow(`SELECT session_id,turn_id,correlation_method FROM telemetry_events WHERE trace_id='trace-1' AND span_id='model-span'`).Scan(&sessionID, &turnID, &method); err != nil {
				t.Fatal(err)
			}
			if sessionID != "codex:session-1" || turnID != "codex:turn-1" || method != "trace_span" {
				t.Fatalf("event association = %q/%q by %q, want codex IDs associated by trace_span", sessionID, turnID, method)
			}
			var eventCount, inputTokens, outputTokens int64
			if err := db.QueryRow(`SELECT event_count,input_tokens,output_tokens FROM sessions WHERE session_id=?`, sessionID).Scan(&eventCount, &inputTokens, &outputTokens); err != nil {
				t.Fatal(err)
			}
			if eventCount != 1 || inputTokens != 7 || outputTokens != 3 {
				t.Fatalf("session aggregates = count %d, input %d, output %d; want 1/7/3", eventCount, inputTokens, outputTokens)
			}
			var turnSessionID sql.NullString
			if err := db.QueryRow(`SELECT session_id FROM turns WHERE turn_id=?`, turnID).Scan(&turnSessionID); err != nil {
				t.Fatal(err)
			}
			if !turnSessionID.Valid || turnSessionID.String != sessionID {
				t.Fatalf("turn session association = %v, want %q", turnSessionID, sessionID)
			}
			unassigned, err := store.ListUnassignedEvents(context.Background(), 10, 0)
			if err != nil || len(unassigned) != 0 {
				t.Fatalf("unassigned events = %+v, error = %v; exact trace/span match should associate it", unassigned, err)
			}
		})
	}
}

func TestSessionTimelineUsesCanonicalSpanInsteadOfSyntheticTraceEvent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)
	startAt := int64(100)
	attributes := map[string]any{
		"conversation.id": "session-1",
		"turn.id":         "turn-1",
		"tool.name":       "exec_command",
		"tool.call_id":    "call-1",
		"process.pid":     int64(456),
	}
	err = codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		Signal: telemetry.SignalTraces, ReceivedAtNS: 210,
		Events: []telemetry.Event{{
			TraceID: "trace-1", SpanID: "tool-span", EventTimeNS: &startAt,
			Name: "span", Attributes: attributes,
		}},
		Spans: []telemetry.Span{{
			TraceID: "trace-1", SpanID: "tool-span", StartTimeNS: 100,
			EndTimeNS: 200, StatusCode: 1, Attributes: attributes,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := store.SessionTimeline(context.Background(), "codex:session-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("timeline returned %d duplicate records, want one canonical span: %+v", len(items), items)
	}
	item := items[0]
	if item.RecordType != "span" || item.SpanRole != "tool" || item.ToolName != "exec_command" ||
		item.ToolStatus == nil || *item.ToolStatus != "success" || item.ProcessPID == nil || *item.ProcessPID != 456 {
		t.Fatalf("timeline lost canonical tool outcome or process PID: %+v", item)
	}
}

func TestTraceSpanAssociationKeepsReceiptOnlyTimePartial(t *testing.T) {
	for _, eventFirst := range []bool{true, false} {
		name := "span-first"
		if eventFirst {
			name = "event-first"
		}
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if err := Initialize(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			writer := codexadapter.NewWriter(NewSQLite(db))
			events := telemetry.Batch{Signal: telemetry.SignalLogs, ReceivedAtNS: 110, Events: []telemetry.Event{{
				TraceID: "trace-1", SpanID: "model-span", Name: "completion",
			}}}
			spans := telemetry.Batch{Signal: telemetry.SignalTraces, ReceivedAtNS: 120, Spans: []telemetry.Span{{
				TraceID: "trace-1", SpanID: "model-span", StartTimeNS: 90, EndTimeNS: 105,
				Attributes: map[string]any{"conversation.id": "session-1"},
			}}}
			if eventFirst {
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
			}
			var eventTime sql.NullInt64
			var eventQuality, sessionQuality string
			if err := db.QueryRow(`SELECT event_time_ns,data_quality FROM telemetry_events WHERE trace_id='trace-1' AND span_id='model-span'`).Scan(&eventTime, &eventQuality); err != nil {
				t.Fatal(err)
			}
			if eventTime.Valid || eventQuality != "partial" {
				t.Fatalf("receipt-only event time was represented as observed: time=%v quality=%q", eventTime, eventQuality)
			}
			var startedAt, lastActivity sql.NullInt64
			if err := db.QueryRow(`SELECT started_at_ns,last_activity_ns,data_quality FROM sessions WHERE session_id='codex:session-1'`).Scan(&startedAt, &lastActivity, &sessionQuality); err != nil {
				t.Fatal(err)
			}
			if !startedAt.Valid || startedAt.Int64 != 90 || !lastActivity.Valid || lastActivity.Int64 != 110 || sessionQuality != "complete" {
				t.Fatalf("session observed/fallback times and quality = %v/%v/%q, want 90/110/complete", startedAt, lastActivity, sessionQuality)
			}
		})
	}
}

func TestTraceSpanRelationshipDoesNotOverrideConflictingSessionIDs(t *testing.T) {
	for _, eventFirst := range []bool{true, false} {
		name := "span-first"
		if eventFirst {
			name = "event-first"
		}
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if err := Initialize(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			writer := codexadapter.NewWriter(NewSQLite(db))
			eventAt := int64(100)
			events := telemetry.Batch{Signal: telemetry.SignalLogs, ReceivedAtNS: 110, Events: []telemetry.Event{{
				TraceID: "trace-1", SpanID: "tool-span", EventTimeNS: &eventAt, Name: "tool",
				Attributes: map[string]any{"conversation.id": "explicit-session", "tool_name": "shell", "tool.call_id": "call-conflict", "success": true},
			}}}
			spans := telemetry.Batch{Signal: telemetry.SignalTraces, ReceivedAtNS: 120, Spans: []telemetry.Span{{
				TraceID: "trace-1", SpanID: "tool-span", StartTimeNS: 90, EndTimeNS: 105,
				Attributes: map[string]any{"conversation.id": "different-session", "tool.name": "shell", "tool.call_id": "call-conflict"},
			}}}
			if eventFirst {
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
			}
			var sessionID, method string
			if err := db.QueryRow(`SELECT session_id,correlation_method FROM telemetry_events WHERE trace_id='trace-1' AND span_id='tool-span'`).Scan(&sessionID, &method); err != nil {
				t.Fatal(err)
			}
			if sessionID != "codex:explicit-session" || method != "observed_identifier" {
				t.Fatalf("conflicting span overwrote explicit event association: session=%q method=%q", sessionID, method)
			}
			var explicitCallSession, spanCallSession sql.NullString
			if err := db.QueryRow(`SELECT session_id FROM tool_calls WHERE tool_call_id='codex:explicit-session:tool:call-conflict'`).Scan(&explicitCallSession); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT session_id FROM tool_calls WHERE tool_call_id='codex:different-session:tool:call-conflict'`).Scan(&spanCallSession); err != nil {
				t.Fatal(err)
			}
			if !explicitCallSession.Valid || explicitCallSession.String != "codex:explicit-session" || !spanCallSession.Valid || spanCallSession.String != "codex:different-session" {
				t.Fatalf("conflicting source tool calls were merged or overwritten: event=%v span=%v", explicitCallSession, spanCallSession)
			}
		})
	}
}

func TestToolCallsFromEventAndSpanMergeByTraceSpanAcrossArrivalOrders(t *testing.T) {
	for _, eventFirst := range []bool{true, false} {
		name := "span-first"
		if eventFirst {
			name = "event-first"
		}
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if err := Initialize(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			writer := codexadapter.NewWriter(NewSQLite(db))
			eventAt := int64(100)
			events := telemetry.Batch{Signal: telemetry.SignalLogs, ReceivedAtNS: 110, Events: []telemetry.Event{{
				TraceID: "trace-1", SpanID: "shell-span", EventTimeNS: &eventAt, Name: "tool_call",
				Attributes: map[string]any{"tool_name": "shell", "tool.call_id": "call-1", "success": true},
			}}}
			spans := telemetry.Batch{Signal: telemetry.SignalTraces, ReceivedAtNS: 120, Spans: []telemetry.Span{{
				TraceID: "trace-1", SpanID: "shell-span", StartTimeNS: 90, EndTimeNS: 105, StatusCode: 1,
				Attributes: map[string]any{"conversation.id": "session-1", "turn.id": "turn-1", "tool.name": "shell", "tool.call_id": "call-1"},
			}}}
			if eventFirst {
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writer.SaveTelemetry(context.Background(), spans); err != nil {
					t.Fatal(err)
				}
				if err := writer.SaveTelemetry(context.Background(), events); err != nil {
					t.Fatal(err)
				}
			}
			var callCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE source_id='codex' AND external_tool_call_id='call-1' AND trace_id='trace-1' AND span_id='shell-span'`).Scan(&callCount); err != nil {
				t.Fatal(err)
			}
			if callCount != 1 {
				t.Fatalf("matching event and span produced %d tool calls, want one", callCount)
			}
			var sessionID, turnID string
			if err := db.QueryRow(`SELECT session_id,turn_id FROM tool_calls WHERE source_id='codex' AND external_tool_call_id='call-1' AND trace_id='trace-1' AND span_id='shell-span'`).Scan(&sessionID, &turnID); err != nil {
				t.Fatal(err)
			}
			if sessionID != "codex:session-1" || turnID != "codex:turn-1" {
				t.Fatalf("merged tool call association = %q/%q, want codex session and turn", sessionID, turnID)
			}
		})
	}
}

func TestListProcessToolSpansReadsOnlyOverlappingToolSpans(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := NewSQLite(db)
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		Signal: telemetry.SignalTraces, ReceivedAtNS: 250,
		Spans: []telemetry.Span{
			{TraceID: "trace", SpanID: "shell-span", StartTimeNS: 100, EndTimeNS: 200, Attributes: map[string]any{
				"tool.name": "shell", "tool.call_id": "call-1", "process.pid": int64(44), "process.parent_pid": int64(10),
			}},
			{TraceID: "trace", SpanID: "old-span", StartTimeNS: 10, EndTimeNS: 20, Attributes: map[string]any{"tool.name": "shell"}},
			{TraceID: "trace", SpanID: "model-span", StartTimeNS: 100, EndTimeNS: 200, Attributes: map[string]any{"model": "gpt"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	spans, err := store.ListProcessToolSpans(context.Background(), 150, 250)
	if err != nil || len(spans) != 1 {
		t.Fatalf("tool spans = %+v, error = %v", spans, err)
	}
	span := spans[0]
	if span.SpanID != "shell-span" || span.ToolCallID != "call-1" || span.ToolName != "shell" || span.ProcessPID == nil || *span.ProcessPID != 44 || span.ParentProcessPID == nil || *span.ParentProcessPID != 10 {
		t.Fatalf("stored tool span fields were not mapped: %+v", span)
	}
}
