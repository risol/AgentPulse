// Package telemetry contains protocol-independent records used between
// telemetry adapters and the persistence layer.
package telemetry

type Signal string

const (
	SignalLogs                    Signal = "logs"
	SignalMetrics                 Signal = "metrics"
	SignalTraces                  Signal = "traces"
	SourceCodex                          = "codex"
	DefaultTelemetryRetentionDays        = 30
)

// Batch is one successfully decoded OTLP export. Zero event timestamps remain
// absent so storage can preserve unavailable time instead of inventing one.
type Batch struct {
	SourceID              string
	Signal                Signal
	ReceivedAtNS          int64
	Events                []Event
	Spans                 []Span
	Metrics               []Metric
	ObservedAttributeKeys []string
	EventMetadata         []CanonicalMetadata
	SpanMetadata          []CanonicalMetadata
}

// CanonicalMetadata is source-normalized association and analysis data. Its
// slice position corresponds to the event/span at the same index in Batch.
type CanonicalMetadata struct {
	SourceID              string
	ExternalSessionID     string
	SessionID             string
	ExternalTurnID        string
	TurnID                string
	CorrelationMethod     string
	CorrelationConfidence *float64
	DataQuality           string
	Model                 string
	SpanRole              string
	ActivityAtNS          *int64
	InputTokens           *int64
	OutputTokens          *int64
	ReasoningTokens       *int64
	TTFTMS                *float64
	ProcessPID            *int64
	ParentProcessPID      *int64
	ToolCall              *ToolCallObservation
}

type ToolCallObservation struct {
	ExternalID  string
	Name        string
	Type        string
	StartTimeNS *int64
	EndTimeNS   *int64
	DurationMS  *float64
	Status      string
	RetryIndex  *int64
	TraceID     string
	SpanID      string
}

type Event struct {
	EventTimeNS *int64
	TraceID     string
	SpanID      string
	Name        string
	Attributes  map[string]any
}

type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	StartTimeNS  int64
	EndTimeNS    int64
	StatusCode   int32
	Attributes   map[string]any
}

type Metric struct {
	Name        string
	Type        string
	StartTimeNS *int64
	TimeNS      *int64
	Value       float64
	Unit        string
	Temporality string
	Monotonic   *bool
	Aggregation MetricAggregation
	Attributes  map[string]any
}

// MetricAggregation preserves numeric distribution fields from OTLP
// histogram, exponential histogram, and summary data points. Exemplars are
// intentionally omitted because local analytics does not consume their trace data.
type MetricAggregation struct {
	Count          *uint64          `json:"count,omitempty"`
	Sum            *float64         `json:"sum,omitempty"`
	Min            *float64         `json:"min,omitempty"`
	Max            *float64         `json:"max,omitempty"`
	ExplicitBounds []float64        `json:"explicit_bounds,omitempty"`
	BucketCounts   []uint64         `json:"bucket_counts,omitempty"`
	Scale          *int32           `json:"scale,omitempty"`
	ZeroCount      *uint64          `json:"zero_count,omitempty"`
	ZeroThreshold  *float64         `json:"zero_threshold,omitempty"`
	Positive       *MetricBuckets   `json:"positive,omitempty"`
	Negative       *MetricBuckets   `json:"negative,omitempty"`
	Quantiles      []MetricQuantile `json:"quantiles,omitempty"`
	Flags          uint32           `json:"flags,omitempty"`
}

type MetricBuckets struct {
	Offset       int32    `json:"offset"`
	BucketCounts []uint64 `json:"bucket_counts"`
}

type MetricQuantile struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}

type SessionSummary struct {
	ID              string
	SourceID        string
	ExternalID      string
	StartedAtNS     *int64
	LastActivityNS  *int64
	Model           string
	EventCount      int64
	ToolCallCount   int64
	CommandCount    int64
	FailedCallCount int64
	InputTokens     *int64
	OutputTokens    *int64
	ReasoningTokens *int64
	DataQuality     string
}

type TimelineRecord struct {
	ID                int64
	RecordType        string
	SourceID          string
	SessionID         *string
	TurnID            *string
	Signal            Signal
	EventTimeNS       *int64
	ReceivedAtNS      int64
	Name              string
	DataQuality       string
	Attributes        map[string]any
	SpanID            string
	ParentSpanID      string
	SpanRole          string
	Model             string
	ToolCallID        *string
	ToolName          string
	ToolStatus        *string
	RetryIndex        *int64
	StartTimeNS       *int64
	EndTimeNS         *int64
	DurationMS        *float64
	ProcessPID        *int64
	ParentProcessPID  *int64
	CorrelationMethod string
}

type ToolSummary struct {
	ToolName        string   `json:"tool_name"`
	TotalCalls      int64    `json:"total_calls"`
	SuccessCalls    int64    `json:"success_calls"`
	FailureCalls    int64    `json:"failure_calls"`
	UnknownOutcomes int64    `json:"unknown_outcomes"`
	OutcomeSamples  int64    `json:"outcome_samples"`
	FailureRate     *float64 `json:"failure_rate"`
	DurationSamples int64    `json:"duration_samples"`
	P50DurationMS   *float64 `json:"p50_duration_ms"`
	P95DurationMS   *float64 `json:"p95_duration_ms"`
	DataQuality     string   `json:"data_quality"`
}

// ToolCallRecord is a privacy-safe view of one persisted tool invocation.
// Prompt and tool input/output content are deliberately omitted.
type ToolCallRecord struct {
	ID                    string   `json:"id"`
	SourceID              string   `json:"source_id"`
	ExternalID            *string  `json:"external_id"`
	SessionID             *string  `json:"session_id"`
	TurnID                *string  `json:"turn_id"`
	ToolName              string   `json:"tool_name"`
	ToolType              *string  `json:"tool_type"`
	StartedAtNS           *int64   `json:"started_at_ns"`
	EndedAtNS             *int64   `json:"ended_at_ns"`
	DurationMS            *float64 `json:"duration_ms"`
	Status                *string  `json:"status"`
	RetryIndex            *int64   `json:"retry_index"`
	TraceID               *string  `json:"trace_id"`
	SpanID                *string  `json:"span_id"`
	SourceEventID         *int64   `json:"source_event_id"`
	CorrelationMethod     string   `json:"correlation_method"`
	CorrelationConfidence *float64 `json:"correlation_confidence"`
	DataQuality           string   `json:"data_quality"`
}

type AnalyticsTrendBucket struct {
	StartedAtNS           int64    `json:"started_at_ns"`
	HasData               bool     `json:"has_data"`
	InputTokens           *int64   `json:"input_tokens"`
	InputTokenSamples     int64    `json:"input_token_samples"`
	OutputTokens          *int64   `json:"output_tokens"`
	OutputTokenSamples    int64    `json:"output_token_samples"`
	TTFTSamples           int64    `json:"ttft_samples"`
	AverageTTFTMS         *float64 `json:"average_ttft_ms"`
	RequestRateSamples    int64    `json:"request_rate_samples"`
	AverageRequestRateTPS *float64 `json:"average_request_rate_tokens_per_second"`
	ToolCallCount         int64    `json:"tool_call_count"`
	DurationSamples       int64    `json:"duration_samples"`
	AverageDurationMS     *float64 `json:"average_duration_ms"`
}

type IngestDiagnostic struct {
	ID           int64  `json:"id"`
	Signal       Signal `json:"signal"`
	Code         string `json:"code"`
	OccurredAtNS int64  `json:"occurred_at_ns"`
}

type TimeInterval struct {
	StartNS int64
	EndNS   int64
}

type SessionAnalyticsInput struct {
	SessionID               string
	SourceID                string
	ExternalID              string
	StartedAtNS             *int64
	LastActivityNS          *int64
	InputTokens             *int64
	OutputTokens            *int64
	ReasoningTokens         *int64
	TTFTMS                  *float64
	TTFTSamples             int64
	LLMIntervals            []TimeInterval
	CommandIntervals        []TimeInterval
	SourceEventIDs          []int64
	SourceEventIDsTruncated bool
	DataQuality             string
}
