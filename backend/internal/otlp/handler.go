package otlp

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/ingest"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const maxRequestBytes = 4 << 20
const maxConcurrentExports = 8

type Handler struct {
	store        Persistence
	writer       BatchWriter
	logger       *slog.Logger
	changes      *telemetry.ChangeBus
	active       chan struct{}
	backpressure atomic.Uint64
}

type Persistence interface {
	Ping(context.Context) error
	SchemaVersion() int
	SaveTelemetry(context.Context, telemetry.Batch) error
}

type BatchWriter interface {
	SaveTelemetry(context.Context, telemetry.Batch) error
}

type DiagnosticRecorder interface {
	RecordIngestDiagnostic(context.Context, telemetry.Signal, string) error
}

type QueueStats interface {
	Depth() int
	Capacity() int
	Rejected() uint64
}

func NewHandler(store Persistence, logger *slog.Logger) http.Handler {
	return NewHandlerWithChanges(store, logger, nil)
}

func NewHandlerWithChanges(store Persistence, logger *slog.Logger, changes *telemetry.ChangeBus) http.Handler {
	return NewHandlerWithWriter(store, store, logger, changes)
}

func NewHandlerWithWriter(store Persistence, writer BatchWriter, logger *slog.Logger, changes *telemetry.ChangeBus) http.Handler {
	if writer == nil {
		writer = store
	}
	h := &Handler{store: store, writer: writer, logger: logger, changes: changes, active: make(chan struct{}, maxConcurrentExports)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /v1/logs", h.logs)
	mux.HandleFunc("POST /v1/metrics", h.metrics)
	mux.HandleFunc("POST /v1/traces", h.traces)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		http.Error(w, `{"status":"error","code":"storage_unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	status := map[string]any{
		"status": "ready", "schema_version": h.store.SchemaVersion(),
		"ingest_backpressure_count": h.backpressure.Load(),
	}
	if queue, okay := h.writer.(QueueStats); okay {
		status["ingest_queue_depth"] = queue.Depth()
		status["ingest_queue_capacity"] = queue.Capacity()
		status["ingest_queue_rejected"] = queue.Rejected()
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	if !h.acquire(w) {
		return
	}
	defer h.release()
	var request collectorlogs.ExportLogsServiceRequest
	if !h.decode(w, r, &request, telemetry.SignalLogs) {
		return
	}
	received := time.Now().UnixNano()
	batch := telemetry.Batch{SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: received}
	for _, resourceLogs := range request.ResourceLogs {
		if resourceLogs.Resource != nil {
			batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(resourceLogs.Resource.Attributes)...)
		}
		for _, scopeLogs := range resourceLogs.ScopeLogs {
			for _, record := range scopeLogs.LogRecords {
				batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(record.Attributes)...)
				attrs := safeAttributes(record.Attributes)
				traceID, spanID := idString(record.TraceId), idString(record.SpanId)
				batch.Events = append(batch.Events, telemetry.Event{
					EventTimeNS: timestamp(record.TimeUnixNano), TraceID: traceID, SpanID: spanID,
					Name: "log_record", Attributes: attrs,
				})
			}
		}
	}
	if err := h.writer.SaveTelemetry(r.Context(), batch); err != nil {
		h.storageError(w, r, telemetry.SignalLogs, err)
		return
	}
	h.notify(telemetry.SignalLogs, received)
	h.respond(w, &collectorlogs.ExportLogsServiceResponse{}, len(batch.Events))
}

func (h *Handler) traces(w http.ResponseWriter, r *http.Request) {
	if !h.acquire(w) {
		return
	}
	defer h.release()
	var request collectortrace.ExportTraceServiceRequest
	if !h.decode(w, r, &request, telemetry.SignalTraces) {
		return
	}
	received := time.Now().UnixNano()
	batch := telemetry.Batch{SourceID: telemetry.SourceCodex, Signal: telemetry.SignalTraces, ReceivedAtNS: received}
	for _, resourceSpans := range request.ResourceSpans {
		if resourceSpans.Resource != nil {
			batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(resourceSpans.Resource.Attributes)...)
		}
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(span.Attributes)...)
				traceID, spanID, parentID := idString(span.TraceId), idString(span.SpanId), idString(span.ParentSpanId)
				attrs := safeAttributes(span.Attributes)
				batch.Spans = append(batch.Spans, telemetry.Span{
					TraceID: traceID, SpanID: spanID, ParentSpanID: parentID,
					StartTimeNS: int64(span.StartTimeUnixNano), EndTimeNS: int64(span.EndTimeUnixNano),
					StatusCode: spanStatusCode(span), Attributes: attrs,
				})
				batch.Events = append(batch.Events, telemetry.Event{
					EventTimeNS: timestamp(span.StartTimeUnixNano), TraceID: traceID, SpanID: spanID,
					Name: "span", Attributes: attrs,
				})
			}
		}
	}
	if err := h.writer.SaveTelemetry(r.Context(), batch); err != nil {
		h.storageError(w, r, telemetry.SignalTraces, err)
		return
	}
	h.notify(telemetry.SignalTraces, received)
	h.respond(w, &collectortrace.ExportTraceServiceResponse{}, len(batch.Spans))
}

func spanStatusCode(span *trace.Span) int32 {
	if span.GetStatus() == nil {
		return 0
	}
	return int32(span.GetStatus().Code)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	if !h.acquire(w) {
		return
	}
	defer h.release()
	var request collectormetrics.ExportMetricsServiceRequest
	if !h.decode(w, r, &request, telemetry.SignalMetrics) {
		return
	}
	received := time.Now().UnixNano()
	batch := telemetry.Batch{SourceID: telemetry.SourceCodex, Signal: telemetry.SignalMetrics, ReceivedAtNS: received}
	for _, resourceMetrics := range request.ResourceMetrics {
		if resourceMetrics.Resource != nil {
			batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(resourceMetrics.Resource.Attributes)...)
		}
		for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
			for _, metric := range scopeMetrics.Metrics {
				metricName := safeName(metric.Name)
				if metric.GetGauge() != nil {
					for _, point := range metric.GetGauge().DataPoints {
						batch.Metrics = append(batch.Metrics, metricRecord(metricName, "gauge", point.StartTimeUnixNano, point.TimeUnixNano, numberValue(point), metric.Unit, point.Attributes))
						batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(point.Attributes)...)
					}
				}
				if metric.GetSum() != nil {
					for _, point := range metric.GetSum().DataPoints {
						record := metricRecord(metricName, "sum", point.StartTimeUnixNano, point.TimeUnixNano, numberValue(point), metric.Unit, point.Attributes)
						record.Temporality = metricTemporality(metric.GetSum().AggregationTemporality)
						monotonic := metric.GetSum().IsMonotonic
						record.Monotonic = &monotonic
						batch.Metrics = append(batch.Metrics, record)
						batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(point.Attributes)...)
					}
				}
				if metric.GetHistogram() != nil {
					for _, point := range metric.GetHistogram().DataPoints {
						attrs := safeAttributes(point.Attributes)
						record := metricRecord(metricName, "histogram", point.StartTimeUnixNano, point.TimeUnixNano, float64(point.Count), metric.Unit, point.Attributes)
						record.Temporality = metricTemporality(metric.GetHistogram().AggregationTemporality)
						record.Aggregation = telemetry.MetricAggregation{
							Count: &point.Count, Sum: point.Sum, Min: point.Min, Max: point.Max,
							ExplicitBounds: point.ExplicitBounds, BucketCounts: point.BucketCounts, Flags: point.Flags,
						}
						batch.Metrics = append(batch.Metrics, record)
						batch.Events = append(batch.Events, metricEvent("histogram:"+metricName, point.TimeUnixNano, attrs))
						batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(point.Attributes)...)
					}
				}
				if metric.GetExponentialHistogram() != nil {
					for _, point := range metric.GetExponentialHistogram().DataPoints {
						attrs := safeAttributes(point.Attributes)
						record := metricRecord(metricName, "exponential_histogram", point.StartTimeUnixNano, point.TimeUnixNano, float64(point.Count), metric.Unit, point.Attributes)
						record.Temporality = metricTemporality(metric.GetExponentialHistogram().AggregationTemporality)
						record.Aggregation = telemetry.MetricAggregation{
							Count: &point.Count, Sum: point.Sum, Min: point.Min, Max: point.Max, Scale: &point.Scale,
							ZeroCount: &point.ZeroCount, ZeroThreshold: &point.ZeroThreshold, Flags: point.Flags,
							Positive: metricBuckets(point.Positive), Negative: metricBuckets(point.Negative),
						}
						batch.Metrics = append(batch.Metrics, record)
						batch.Events = append(batch.Events, metricEvent("exponential_histogram:"+metricName, point.TimeUnixNano, attrs))
						batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(point.Attributes)...)
					}
				}
				if metric.GetSummary() != nil {
					for _, point := range metric.GetSummary().DataPoints {
						attrs := safeAttributes(point.Attributes)
						record := metricRecord(metricName, "summary", point.StartTimeUnixNano, point.TimeUnixNano, float64(point.Count), metric.Unit, point.Attributes)
						record.Aggregation.Count = &point.Count
						record.Aggregation.Sum = &point.Sum
						record.Aggregation.Flags = point.Flags
						for _, value := range point.QuantileValues {
							if value != nil {
								record.Aggregation.Quantiles = append(record.Aggregation.Quantiles, telemetry.MetricQuantile{Quantile: value.Quantile, Value: value.Value})
							}
						}
						batch.Metrics = append(batch.Metrics, record)
						batch.Events = append(batch.Events, metricEvent("summary:"+metricName, point.TimeUnixNano, attrs))
						batch.ObservedAttributeKeys = append(batch.ObservedAttributeKeys, attributeKeys(point.Attributes)...)
					}
				}
			}
		}
	}
	if err := h.writer.SaveTelemetry(r.Context(), batch); err != nil {
		h.storageError(w, r, telemetry.SignalMetrics, err)
		return
	}
	h.notify(telemetry.SignalMetrics, received)
	h.respond(w, &collectormetrics.ExportMetricsServiceResponse{}, len(batch.Metrics))
}

func (h *Handler) notify(signal telemetry.Signal, receivedAtNS int64) {
	if h.changes != nil {
		h.changes.Publish(telemetry.Update{Signal: signal, ReceivedAtNS: receivedAtNS})
	}
}

func (h *Handler) decode(w http.ResponseWriter, r *http.Request, message proto.Message, signal telemetry.Signal) bool {
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/x-protobuf") && !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/protobuf") {
		h.recordDiagnostic(r.Context(), signal, "unsupported_content_type")
		http.Error(w, `{"error":"unsupported_content_type"}`, http.StatusUnsupportedMediaType)
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			h.recordDiagnostic(r.Context(), signal, "request_too_large")
			http.Error(w, `{"error":"request_too_large"}`, http.StatusRequestEntityTooLarge)
		} else {
			h.recordDiagnostic(r.Context(), signal, "invalid_request_body")
			http.Error(w, `{"error":"invalid_request_body"}`, http.StatusBadRequest)
		}
		return false
	}
	if err := proto.Unmarshal(body, message); err != nil {
		h.recordDiagnostic(r.Context(), signal, "invalid_protobuf")
		http.Error(w, `{"error":"invalid_protobuf"}`, http.StatusBadRequest)
		return false
	}
	return true
}

func (h *Handler) respond(w http.ResponseWriter, response proto.Message, count int) {
	body, err := proto.Marshal(response)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("OTLP response encoding failed", "error", err)
		}
		http.Error(w, `{"error":"response_encoding_failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("X-AgentPulse-Records", strconv.Itoa(count))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(body))
}

func (h *Handler) storageError(w http.ResponseWriter, r *http.Request, signal telemetry.Signal, err error) {
	if errors.Is(err, ingest.ErrQueueFull) {
		h.backpressure.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"receiver_queue_full"}`, http.StatusServiceUnavailable)
		return
	}
	if h.logger != nil {
		if code, okay := databaseErrorCode(err); okay {
			h.logger.Error("OTLP persistence failed", "database_code", code, "storage_error_class", storageErrorClass(err))
		} else {
			h.logger.Error("OTLP persistence failed", "storage_error_class", storageErrorClass(err))
		}
	}
	h.recordDiagnostic(r.Context(), signal, "storage_failure")
	http.Error(w, `{"error":"storage_failure"}`, http.StatusServiceUnavailable)
}

func (h *Handler) recordDiagnostic(ctx context.Context, signal telemetry.Signal, code string) {
	recorder, okay := h.store.(DiagnosticRecorder)
	if !okay {
		return
	}
	if err := recorder.RecordIngestDiagnostic(ctx, signal, code); err != nil && h.logger != nil {
		if databaseCode, okay := databaseErrorCode(err); okay {
			h.logger.Warn("could not persist ingest diagnostic", "signal", signal, "code", code, "database_code", databaseCode, "storage_error_class", storageErrorClass(err))
		} else {
			h.logger.Warn("could not persist ingest diagnostic", "signal", signal, "code", code, "storage_error_class", storageErrorClass(err))
		}
	}
}

func storageErrorClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	if code, okay := databaseErrorCode(err); okay {
		switch code {
		case 5, 6, 261, 262, 517, 518:
			return "sqlite_busy"
		case 9:
			return "sqlite_interrupted"
		case 10, 11, 13, 14:
			return "sqlite_io"
		case 19, 275, 787, 1299, 1555, 2067:
			return "sqlite_constraint"
		default:
			return "sqlite"
		}
	}
	return "other"
}

func databaseErrorCode(err error) (int, bool) {
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		return coded.Code(), true
	}
	return 0, false
}

func (h *Handler) acquire(w http.ResponseWriter) bool {
	select {
	case h.active <- struct{}{}:
		return true
	default:
		h.backpressure.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"receiver_busy"}`, http.StatusServiceUnavailable)
		return false
	}
}

func (h *Handler) release() {
	<-h.active
}

func idString(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return hex.EncodeToString(value)
}

func timestamp(value uint64) *int64 {
	if value == 0 {
		return nil
	}
	converted := int64(value)
	return &converted
}

func metricRecord(name, metricType string, startTime, eventTime uint64, value float64, unit string, attributes []*common.KeyValue) telemetry.Metric {
	return telemetry.Metric{
		Name: name, Type: metricType, StartTimeNS: timestamp(startTime), TimeNS: timestamp(eventTime), Value: value,
		Unit: safeName(unit), Attributes: safeAttributes(attributes),
	}
}

func metricTemporality(value metrics.AggregationTemporality) string {
	switch value {
	case metrics.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA:
		return "delta"
	case metrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE:
		return "cumulative"
	default:
		return "unspecified"
	}
}

func metricBuckets(value *metrics.ExponentialHistogramDataPoint_Buckets) *telemetry.MetricBuckets {
	if value == nil {
		return nil
	}
	return &telemetry.MetricBuckets{Offset: value.Offset, BucketCounts: value.BucketCounts}
}

func metricEvent(name string, eventTime uint64, attributes map[string]any) telemetry.Event {
	return telemetry.Event{Name: name, EventTimeNS: timestamp(eventTime), Attributes: attributes}
}

func attributeKeys(attributes []*common.KeyValue) []string {
	keys := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		if attribute != nil {
			keys = append(keys, attribute.Key)
		}
	}
	return keys
}

func numberValue(point *metrics.NumberDataPoint) float64 {
	if point.GetAsInt() != 0 {
		return float64(point.GetAsInt())
	}
	return point.GetAsDouble()
}

var safeAttributeKeys = map[string]bool{
	"gen_ai.operation.name": true, "gen_ai.request.model": true, "gen_ai.response.model": true,
	"gen_ai.system": true, "gen_ai.usage.input_tokens": true, "gen_ai.usage.output_tokens": true,
	"gen_ai.usage.cache_read.input_tokens": true, "gen_ai.usage.cache_write.input_tokens": true,
	"codex.usage.reasoning_output_tokens": true, "codex.usage.total_tokens": true,
	"conversation.id": true, "turn.id": true, "session.id": true, "thread.id": true,
	"model": true, "model_reasoning_effort": true, "codex.request.reasoning_effort": true,
	"codex.turn.phase": true, "tool.name": true, "tool_name": true, "tool_namespace": true,
	"tool.call_id": true, "call_id": true, "tool.type": true, "retry_index": true, "error.type": true,
	"input_token_count": true, "output_token_count": true, "reasoning_token_count": true,
	"cached_token_count": true, "cache_write_token_count": true, "tool_token_count": true,
	"ttft_ms": true, "duration_ms": true, "success": true, "outcome": true, "stop_reason": true,
	"event.name": true, "event.kind": true, "originator": true, "agent_name": true,
	"provider": true, "wire_api": true, "service.name": true, "service.version": true,
	"process.pid": true, "process.parent_pid": true,
	"app.version": true, "telemetry.sdk.language": true, "telemetry.sdk.name": true,
	"telemetry.sdk.version": true, "http.request.method": true,
}

func safeAttributes(attributes []*common.KeyValue) map[string]any {
	result := make(map[string]any)
	for _, attribute := range attributes {
		if attribute == nil || !safeAttributeKeys[attribute.Key] || attribute.Value == nil {
			continue
		}
		var value any
		switch typed := attribute.Value.Value.(type) {
		case *common.AnyValue_StringValue:
			value = typed.StringValue
		case *common.AnyValue_BoolValue:
			value = typed.BoolValue
		case *common.AnyValue_IntValue:
			value = typed.IntValue
		case *common.AnyValue_DoubleValue:
			value = typed.DoubleValue
		default:
			continue
		}
		if stringValue, ok := value.(string); ok && len(stringValue) > 256 {
			value = stringValue[:256]
		}
		result[attribute.Key] = value
	}
	return result
}

func safeName(name string) string {
	if len(name) > 128 {
		return name[:128]
	}
	return name
}
