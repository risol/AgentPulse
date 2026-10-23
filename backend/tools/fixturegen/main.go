package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

var fixtureSalt [32]byte

func main() {
	if _, err := rand.Read(fixtureSalt[:]); err != nil {
		fail(err)
	}
	dbPath := flag.String("db", "", "AgentPulse SQLite database path")
	outDir := flag.String("out", "tests/fixtures", "output directory for redacted OTLP Protobuf fixtures")
	flag.Parse()
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "-db is required")
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		fail(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(*dbPath)+"?mode=ro")
	if err != nil {
		fail(err)
	}
	defer db.Close()
	ctx := context.Background()

	logs, err := exportLogs(ctx, db)
	if err != nil {
		fail(err)
	}
	if err := writeProto(filepath.Join(*outDir, "codex-logs.pb"), logs); err != nil {
		fail(err)
	}

	metrics, err := exportMetrics(ctx, db)
	if err != nil {
		fail(err)
	}
	if err := writeProto(filepath.Join(*outDir, "codex-metrics.pb"), metrics); err != nil {
		fail(err)
	}

	traces, err := exportTraces(ctx, db)
	if err != nil {
		fail(err)
	}
	if err := writeProto(filepath.Join(*outDir, "codex-traces.pb"), traces); err != nil {
		fail(err)
	}
	fmt.Printf("Wrote redacted logs, metrics, and traces fixtures to %s\n", *outDir)
}

func exportLogs(ctx context.Context, db *sql.DB) (*collectorlogs.ExportLogsServiceRequest, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_time_ns,trace_id,span_id,attributes_json FROM telemetry_events WHERE signal_type='logs' ORDER BY id LIMIT 8`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*logspb.LogRecord
	for rows.Next() {
		var timestamp sql.NullInt64
		var traceID, spanID sql.NullString
		var raw string
		if err := rows.Scan(&timestamp, &traceID, &spanID, &raw); err != nil {
			return nil, err
		}
		attrs, err := fixtureAttributes(raw)
		if err != nil {
			return nil, err
		}
		record := &logspb.LogRecord{Attributes: attrs}
		if timestamp.Valid && timestamp.Int64 > 0 {
			record.TimeUnixNano = 1_000_000_000
		}
		if traceID.Valid {
			record.TraceId = fixtureID(traceID.String, 16)
		}
		if spanID.Valid {
			record.SpanId = fixtureID(spanID.String, 8)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &collectorlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}}}}}, nil
}

func exportMetrics(ctx context.Context, db *sql.DB) (*collectormetrics.ExportMetricsServiceRequest, error) {
	rows, err := db.QueryContext(ctx, `SELECT metric_name,metric_type,time_ns,numeric_value,unit,attributes_json FROM metric_points ORDER BY id LIMIT 8`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []*metricspb.Metric
	for rows.Next() {
		var name, kind, unit, raw string
		var timestamp sql.NullInt64
		var value sql.NullFloat64
		if err := rows.Scan(&name, &kind, &timestamp, &value, &unit, &raw); err != nil {
			return nil, err
		}
		attrs, err := fixtureAttributes(raw)
		if err != nil {
			return nil, err
		}
		point := &metricspb.NumberDataPoint{Attributes: attrs}
		if timestamp.Valid && timestamp.Int64 > 0 {
			point.TimeUnixNano = 1_000_000_000
		}
		if value.Valid {
			point.Value = &metricspb.NumberDataPoint_AsDouble{AsDouble: value.Float64}
		}
		metric := &metricspb.Metric{Name: name, Unit: unit}
		switch kind {
		case "sum":
			metric.Data = &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: []*metricspb.NumberDataPoint{point}, AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true}}
		default:
			metric.Data = &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &collectormetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}}}}}, nil
}

func exportTraces(ctx context.Context, db *sql.DB) (*collectortrace.ExportTraceServiceRequest, error) {
	rows, err := db.QueryContext(ctx, `SELECT trace_id,span_id,parent_span_id,name,start_time_ns,end_time_ns,status_code,attributes_json FROM spans ORDER BY start_time_ns LIMIT 8`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spans []*tracepb.Span
	for rows.Next() {
		var traceID, spanID, parentID, name, raw string
		var start, end uint64
		var status int32
		if err := rows.Scan(&traceID, &spanID, &parentID, &name, &start, &end, &status, &raw); err != nil {
			return nil, err
		}
		attrs, err := fixtureAttributes(raw)
		if err != nil {
			return nil, err
		}
		// Keep span durations useful while shifting all wall-clock times to an
		// artificial epoch so fixtures do not disclose when the task ran.
		duration := end - start
		spans = append(spans, &tracepb.Span{
			TraceId: fixtureID(traceID, 16), SpanId: fixtureID(spanID, 8), ParentSpanId: fixtureID(parentID, 8),
			Name: "span", StartTimeUnixNano: 1_000_000_000, EndTimeUnixNano: 1_000_000_000 + duration,
			Status: &tracepb.Status{Code: tracepb.Status_StatusCode(status)}, Attributes: attrs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &collectortrace.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}}, nil
}

func fixtureAttributes(raw string) ([]*common.KeyValue, error) {
	values := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	var attributes []*common.KeyValue
	for key, value := range values {
		if isIdentifierKey(key) {
			value = "fixture-id"
		}
		var wrapped *common.AnyValue
		switch typed := value.(type) {
		case string:
			wrapped = &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: typed}}
		case bool:
			wrapped = &common.AnyValue{Value: &common.AnyValue_BoolValue{BoolValue: typed}}
		case float64:
			wrapped = &common.AnyValue{Value: &common.AnyValue_DoubleValue{DoubleValue: typed}}
		default:
			continue
		}
		attributes = append(attributes, &common.KeyValue{Key: key, Value: wrapped})
	}
	return attributes, nil
}

func isIdentifierKey(key string) bool {
	return strings.HasSuffix(key, ".id") || strings.HasSuffix(key, "_id") || key == "call_id" || key == "thread_id"
}

func fixtureID(value string, size int) []byte {
	if value == "" {
		return nil
	}
	input := append(fixtureSalt[:], []byte(value)...)
	digest := sha256.Sum256(input)
	return digest[:size]
}

func writeProto(path string, message proto.Message) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
