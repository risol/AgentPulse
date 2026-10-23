package otlp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/storage"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	collectorMetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func TestRedactedFixturesIngestAndPersist(t *testing.T) {
	db, handler := newTestHandler(t)
	tests := []struct {
		name     string
		endpoint string
		signal   string
		query    string
	}{
		{name: "codex-logs.pb", endpoint: "/v1/logs", signal: "logs", query: "SELECT count(*) FROM telemetry_events WHERE signal_type='logs'"},
		{name: "codex-metrics.pb", endpoint: "/v1/metrics", signal: "metrics", query: "SELECT count(*) FROM metric_points"},
		{name: "codex-traces.pb", endpoint: "/v1/traces", signal: "traces", query: "SELECT count(*) FROM spans"},
	}

	for _, test := range tests {
		t.Run(test.signal, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", test.name))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, test.endpoint, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("fixture returned HTTP %d: %s", response.Code, response.Body.String())
			}
			if count, err := strconv.Atoi(response.Header().Get("X-AgentPulse-Records")); err != nil || count == 0 {
				t.Fatalf("expected a positive record count, got %q", response.Header().Get("X-AgentPulse-Records"))
			}
			var persisted int
			if err := db.QueryRow(test.query).Scan(&persisted); err != nil {
				t.Fatal(err)
			}
			if persisted == 0 {
				t.Fatalf("fixture did not persist any %s records", test.signal)
			}
			var receipt string
			if err := db.QueryRow(`SELECT value FROM app_state WHERE key=?`, "last_received_"+test.signal+"_ns").Scan(&receipt); err != nil {
				t.Fatalf("missing receipt state: %v", err)
			}
		})
	}
}

func TestMetricsPreserveOTLPAggregations(t *testing.T) {
	db, handler := newTestHandler(t)
	histogramSum, histogramMin, histogramMax := 12.5, 1.0, 8.0
	exponentialSum, exponentialMin, exponentialMax := 9.0, -2.0, 4.0
	summarySum := 15.0
	request := &collectorMetrics.ExportMetricsServiceRequest{
		ResourceMetrics: []*metrics.ResourceMetrics{{ScopeMetrics: []*metrics.ScopeMetrics{{Metrics: []*metrics.Metric{
			{
				Name: "requests.total", Data: &metrics.Metric_Sum{Sum: &metrics.Sum{
					AggregationTemporality: metrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					IsMonotonic:            true,
					DataPoints: []*metrics.NumberDataPoint{{
						StartTimeUnixNano: 100, TimeUnixNano: 200,
						Value: &metrics.NumberDataPoint_AsInt{AsInt: 7},
					}},
				}},
			},
			{
				Name: "request.duration", Unit: "ms", Data: &metrics.Metric_Histogram{Histogram: &metrics.Histogram{
					AggregationTemporality: metrics.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
					DataPoints: []*metrics.HistogramDataPoint{{
						StartTimeUnixNano: 300, TimeUnixNano: 400, Count: 3, Sum: &histogramSum,
						Min: &histogramMin, Max: &histogramMax,
						ExplicitBounds: []float64{2, 5}, BucketCounts: []uint64{1, 1, 1},
					}},
				}},
			},
			{
				Name: "request.size", Data: &metrics.Metric_ExponentialHistogram{ExponentialHistogram: &metrics.ExponentialHistogram{
					AggregationTemporality: metrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					DataPoints: []*metrics.ExponentialHistogramDataPoint{{
						StartTimeUnixNano: 500, TimeUnixNano: 600, Count: 3, Sum: &exponentialSum,
						Min: &exponentialMin, Max: &exponentialMax, Scale: 2, ZeroCount: 1, ZeroThreshold: 0.01,
						Positive: &metrics.ExponentialHistogramDataPoint_Buckets{Offset: 3, BucketCounts: []uint64{1}},
						Negative: &metrics.ExponentialHistogramDataPoint_Buckets{Offset: -1, BucketCounts: []uint64{1}},
					}},
				}},
			},
			{
				Name: "request.summary", Data: &metrics.Metric_Summary{Summary: &metrics.Summary{
					DataPoints: []*metrics.SummaryDataPoint{{
						StartTimeUnixNano: 700, TimeUnixNano: 800, Count: 3, Sum: summarySum,
						QuantileValues: []*metrics.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 4}, {Quantile: 0.95, Value: 7}},
					}},
				}},
			},
		}}}}},
	}
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	requestHTTP := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	requestHTTP.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestHTTP)
	if response.Code != http.StatusOK {
		t.Fatalf("metric export returned HTTP %d: %s", response.Code, response.Body.String())
	}

	type storedMetric struct {
		metricType  string
		startTime   sql.NullInt64
		temporality string
		monotonic   sql.NullBool
		aggregation telemetry.MetricAggregation
	}
	stored := make(map[string]storedMetric)
	rows, err := db.Query(`SELECT metric_name,metric_type,start_time_ns,temporality,monotonic,aggregation_json FROM metric_points`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, aggregationJSON string
		var value storedMetric
		if err := rows.Scan(&name, &value.metricType, &value.startTime, &value.temporality, &value.monotonic, &aggregationJSON); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(aggregationJSON), &value.aggregation); err != nil {
			t.Fatal(err)
		}
		stored[name] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := stored["requests.total"]; got.temporality != "cumulative" || !got.monotonic.Valid || !got.monotonic.Bool || !got.startTime.Valid || got.startTime.Int64 != 100 {
		t.Fatalf("sum metadata was not preserved: %+v", got)
	}
	histogram := stored["request.duration"]
	if histogram.metricType != "histogram" || !histogram.startTime.Valid || histogram.startTime.Int64 != 300 || histogram.temporality != "delta" ||
		histogram.aggregation.Count == nil || *histogram.aggregation.Count != 3 || histogram.aggregation.Sum == nil || *histogram.aggregation.Sum != histogramSum ||
		histogram.aggregation.Min == nil || *histogram.aggregation.Min != histogramMin || histogram.aggregation.Max == nil || *histogram.aggregation.Max != histogramMax ||
		len(histogram.aggregation.ExplicitBounds) != 2 || len(histogram.aggregation.BucketCounts) != 3 {
		t.Fatalf("histogram aggregation was not preserved: %+v", histogram)
	}
	exponential := stored["request.size"]
	if exponential.metricType != "exponential_histogram" || exponential.temporality != "cumulative" || exponential.aggregation.Scale == nil || *exponential.aggregation.Scale != 2 ||
		exponential.aggregation.ZeroCount == nil || *exponential.aggregation.ZeroCount != 1 || exponential.aggregation.Positive == nil || exponential.aggregation.Positive.Offset != 3 ||
		exponential.aggregation.Negative == nil || exponential.aggregation.Negative.Offset != -1 {
		t.Fatalf("exponential histogram aggregation was not preserved: %+v", exponential)
	}
	summary := stored["request.summary"]
	if summary.metricType != "summary" || summary.aggregation.Count == nil || *summary.aggregation.Count != 3 || summary.aggregation.Sum == nil || *summary.aggregation.Sum != summarySum ||
		len(summary.aggregation.Quantiles) != 2 || summary.aggregation.Quantiles[1].Quantile != 0.95 || summary.aggregation.Quantiles[1].Value != 7 {
		t.Fatalf("summary aggregation was not preserved: %+v", summary)
	}
}

func TestPublishesChangeAfterTelemetryIsPersisted(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := storage.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	bus := telemetry.NewChangeBus()
	updates, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	handler := NewHandlerWithChanges(
		storage.NewSQLite(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		bus,
	)
	body, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "codex-logs.pb"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("fixture returned HTTP %d: %s", response.Code, response.Body.String())
	}
	update := <-updates
	if update.Signal != telemetry.SignalLogs || update.ReceivedAtNS <= 0 {
		t.Fatalf("unexpected update notification: %+v", update)
	}
	var stored int
	if err := db.QueryRow(`SELECT COUNT(*) FROM telemetry_events WHERE signal_type='logs'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == 0 {
		t.Fatal("notification was published without persisted log events")
	}
}

func TestRejectsMalformedAndOversizedRequests(t *testing.T) {
	db, handler := newTestHandler(t)
	tests := []struct {
		name       string
		body       []byte
		wantStatus int
	}{
		{name: "malformed protobuf", body: []byte{0x0a}, wantStatus: http.StatusBadRequest},
		{name: "oversized body", body: bytes.Repeat([]byte{0}, maxRequestBytes+1), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(test.body))
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("got HTTP %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
	var diagnostics int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ingest_diagnostics WHERE error_code IN ('invalid_protobuf','request_too_large')`).Scan(&diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics != 2 {
		t.Fatalf("persisted %d parse diagnostics, want 2", diagnostics)
	}
}

func TestAcceptsUnknownProtobufFields(t *testing.T) {
	_, handler := newTestHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader([]byte{0x98, 0x06, 0x01}))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("got HTTP %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestConcurrentExportLimitReportsBackpressure(t *testing.T) {
	entered := make(chan struct{}, maxConcurrentExports)
	release := make(chan struct{})
	store := &blockingPersistence{entered: entered, release: release}
	handler := NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var requests sync.WaitGroup
	for range maxConcurrentExports {
		requests.Add(1)
		go func() {
			defer requests.Done()
			request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(nil))
			request.Header.Set("Content-Type", "application/x-protobuf")
			handler.ServeHTTP(httptest.NewRecorder(), request)
		}()
	}
	for range maxConcurrentExports {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			requests.Wait()
			t.Fatal("export requests did not reach storage")
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(nil))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "1" {
		close(release)
		requests.Wait()
		t.Fatalf("busy receiver returned HTTP %d with Retry-After %q", response.Code, response.Header().Get("Retry-After"))
	}

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	var status map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &status); err != nil {
		close(release)
		requests.Wait()
		t.Fatal(err)
	}
	if status["ingest_backpressure_count"] != float64(1) {
		close(release)
		requests.Wait()
		t.Fatalf("backpressure count = %v, want 1", status["ingest_backpressure_count"])
	}
	close(release)
	requests.Wait()
}

type blockingPersistence struct {
	entered chan struct{}
	release <-chan struct{}
}

func (s *blockingPersistence) Ping(context.Context) error { return nil }

func (s *blockingPersistence) SchemaVersion() int { return 2 }

func (s *blockingPersistence) SaveTelemetry(context.Context, telemetry.Batch) error {
	s.entered <- struct{}{}
	<-s.release
	return nil
}

func TestProcessIdentifiersAreAllowlistedWithoutCommandLines(t *testing.T) {
	attributes := safeAttributes([]*common.KeyValue{
		{Key: "process.pid", Value: &common.AnyValue{Value: &common.AnyValue_IntValue{IntValue: 77}}},
		{Key: "process.parent_pid", Value: &common.AnyValue{Value: &common.AnyValue_IntValue{IntValue: 12}}},
		{Key: "process.command_line", Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: "pwsh -Command secret"}}},
	})
	if attributes["process.pid"] != int64(77) || attributes["process.parent_pid"] != int64(12) {
		t.Fatalf("safe process identifiers were dropped: %+v", attributes)
	}
	if _, exists := attributes["process.command_line"]; exists {
		t.Fatalf("sensitive process command line should not be persisted: %+v", attributes)
	}
}

func newTestHandler(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return db, NewHandler(storage.NewSQLite(db), logger)
}
