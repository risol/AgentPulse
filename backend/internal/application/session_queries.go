package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

var (
	ErrInvalidSessionQueryRange = errors.New("invalid session query range")
	ErrInvalidSessionModel      = errors.New("invalid session model filter")
	ErrInvalidToolOutcome       = errors.New("invalid tool outcome filter")
)

type SessionQueryRepository interface {
	ListSessions(context.Context, int, *int64, string, *int64, string) ([]telemetry.SessionSummary, error)
	SessionTimeline(context.Context, string, int) ([]telemetry.TimelineRecord, error)
	ListUnassignedEvents(context.Context, int, int64) ([]telemetry.TimelineRecord, error)
	ListToolSummary(context.Context, int64, int64, string) ([]telemetry.ToolSummary, error)
	ListToolCalls(context.Context, int64, int64, string, string, string, int, *int64, string) ([]telemetry.ToolCallRecord, error)
}

type ToolCallQuery struct {
	Range      string
	SessionID  string
	ToolName   string
	Outcome    string
	Limit      int
	BeforeAtNS *int64
	BeforeID   string
}

type SessionListQuery struct {
	Range      string
	Model      string
	Limit      int
	BeforeAtNS *int64
	BeforeID   string
}

// SessionListItem is the application-facing session list projection. The raw
// activity timestamp remains available to the transport for opaque cursors,
// while the serialized view contains only the stable API fields.
type SessionListItem struct {
	ID                 string   `json:"id"`
	StartedAt          *string  `json:"started_at"`
	LastActivityAt     *string  `json:"last_activity_at"`
	SourceID           string   `json:"source_id"`
	ExternalID         string   `json:"external_id"`
	Model              string   `json:"model"`
	EventCount         int64    `json:"event_count"`
	ActivityDurationMS *float64 `json:"activity_duration_ms"`
	ToolCallCount      int64    `json:"tool_call_count"`
	CommandCount       int64    `json:"command_count"`
	FailedCallCount    int64    `json:"failed_call_count"`
	InputTokens        *int64   `json:"input_tokens"`
	OutputTokens       *int64   `json:"output_tokens"`
	ReasoningTokens    *int64   `json:"reasoning_tokens"`
	Association        string   `json:"association"`
	DataQuality        string   `json:"data_quality"`

	LastActivityNS *int64 `json:"-"`
}

type SessionTimelineItem struct {
	RecordID          string           `json:"record_id"`
	RecordType        string           `json:"record_type"`
	EventID           *int64           `json:"event_id"`
	SourceID          string           `json:"source_id"`
	SessionID         *string          `json:"session_id"`
	TurnID            *string          `json:"turn_id"`
	Signal            telemetry.Signal `json:"signal"`
	Kind              string           `json:"kind"`
	Timestamp         *string          `json:"timestamp"`
	StartAt           *string          `json:"start_at"`
	EndAt             *string          `json:"end_at"`
	SpanID            *string          `json:"span_id"`
	ParentSpanID      *string          `json:"parent_span_id"`
	SpanRole          *string          `json:"span_role"`
	DataQuality       string           `json:"data_quality"`
	CorrelationMethod *string          `json:"correlation_method"`
	Tool              string           `json:"tool"`
	ToolCallID        *string          `json:"tool_call_id"`
	Model             string           `json:"model"`
	Outcome           any              `json:"outcome"`
	RetryIndex        *int64           `json:"retry_index"`
	ProcessPID        *int64           `json:"process_pid"`
	ParentProcessPID  *int64           `json:"parent_process_pid"`
	DurationMS        any              `json:"duration_ms"`
	TTFTMS            any              `json:"ttft_ms"`
	InputTokens       any              `json:"input_tokens"`
	OutputTokens      any              `json:"output_tokens"`
}

type UnassignedEventItem struct {
	EventID     int64            `json:"event_id"`
	SourceID    string           `json:"source_id"`
	Signal      telemetry.Signal `json:"signal"`
	Kind        string           `json:"kind"`
	Timestamp   *string          `json:"timestamp"`
	TurnID      *string          `json:"turn_id"`
	DataQuality string           `json:"data_quality"`
	Tool        string           `json:"tool"`
	Model       string           `json:"model"`
}

// SessionQueryService keeps persistence query composition out of the HTTP transport.
type SessionQueryService struct {
	repository SessionQueryRepository
	now        func() time.Time
}

func NewSessionQueryService(repository SessionQueryRepository) *SessionQueryService {
	return &SessionQueryService{repository: repository, now: time.Now}
}

func (s *SessionQueryService) ListSessions(ctx context.Context, query SessionListQuery) ([]SessionListItem, error) {
	model := strings.TrimSpace(query.Model)
	if len(model) > 128 {
		return nil, ErrInvalidSessionModel
	}
	var sinceNS *int64
	if query.Range != "all" {
		duration, okay := SummaryRangeDuration(query.Range)
		if !okay {
			return nil, ErrInvalidSessionQueryRange
		}
		since := s.now().Add(-duration).UnixNano()
		sinceNS = &since
	}
	records, err := s.repository.ListSessions(ctx, query.Limit, query.BeforeAtNS, query.BeforeID, sinceNS, model)
	if err != nil {
		return nil, err
	}
	items := make([]SessionListItem, 0, len(records))
	for _, record := range records {
		var activityDurationMS *float64
		if record.StartedAtNS != nil && record.LastActivityNS != nil && *record.LastActivityNS >= *record.StartedAtNS {
			duration := float64(*record.LastActivityNS-*record.StartedAtNS) / float64(time.Millisecond)
			activityDurationMS = &duration
		}
		items = append(items, SessionListItem{
			ID: record.ID, StartedAt: formatApplicationTimestamp(record.StartedAtNS),
			LastActivityAt: formatApplicationTimestamp(record.LastActivityNS),
			SourceID:       record.SourceID, ExternalID: record.ExternalID, Model: record.Model,
			EventCount: record.EventCount, ActivityDurationMS: activityDurationMS,
			ToolCallCount: record.ToolCallCount, CommandCount: record.CommandCount,
			FailedCallCount: record.FailedCallCount, InputTokens: record.InputTokens,
			OutputTokens: record.OutputTokens, ReasoningTokens: record.ReasoningTokens,
			Association: "observed_identifier", DataQuality: record.DataQuality,
			LastActivityNS: record.LastActivityNS,
		})
	}
	return items, nil
}

func (s *SessionQueryService) Timeline(ctx context.Context, sessionID string, limit int) ([]SessionTimelineItem, error) {
	records, err := s.repository.SessionTimeline(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]SessionTimelineItem, 0, len(records))
	for _, record := range records {
		items = append(items, mapSessionTimelineRecord(record))
	}
	return items, nil
}

func (s *SessionQueryService) UnassignedEvents(ctx context.Context, limit int, beforeID int64) ([]UnassignedEventItem, error) {
	records, err := s.repository.ListUnassignedEvents(ctx, limit, beforeID)
	if err != nil {
		return nil, err
	}
	items := make([]UnassignedEventItem, 0, len(records))
	for _, record := range records {
		timestamp := record.EventTimeNS
		if timestamp == nil {
			received := record.ReceivedAtNS
			timestamp = &received
		}
		items = append(items, UnassignedEventItem{
			EventID: record.ID, SourceID: record.SourceID, Signal: record.Signal,
			Kind: eventKind(record.Name, record.Attributes), Timestamp: formatApplicationTimestamp(timestamp),
			TurnID: record.TurnID, DataQuality: record.DataQuality,
			Tool:  attributeString(record.Attributes, "tool_name", "tool.name"),
			Model: attributeString(record.Attributes, "model", "gen_ai.response.model", "gen_ai.request.model"),
		})
	}
	return items, nil
}

func mapSessionTimelineRecord(record telemetry.TimelineRecord) SessionTimelineItem {
	timestamp := record.EventTimeNS
	if timestamp == nil {
		received := record.ReceivedAtNS
		timestamp = &received
	}
	kind := eventKind(record.Name, record.Attributes)
	if record.RecordType == "span" {
		switch record.SpanRole {
		case "model":
			kind = "model_request"
		case "tool":
			kind = "tool_call"
		default:
			kind = "span"
		}
	}
	toolName := record.ToolName
	if toolName == "" {
		toolName = attributeString(record.Attributes, "tool_name", "tool.name")
	}
	model := record.Model
	if model == "" {
		model = attributeString(record.Attributes, "model", "gen_ai.response.model", "gen_ai.request.model")
	}
	var outcome any = attributeString(record.Attributes, "outcome", "success", "error.type")
	if record.ToolStatus != nil {
		outcome = *record.ToolStatus
	}
	var duration any = attributeValue(record.Attributes, "duration_ms")
	if record.DurationMS != nil {
		duration = *record.DurationMS
	}
	var eventID *int64
	recordID := "span:" + record.SpanID
	if record.RecordType != "span" {
		eventID = &record.ID
		recordID = "event:" + strconv.FormatInt(record.ID, 10)
	}
	return SessionTimelineItem{
		RecordID: recordID, RecordType: record.RecordType, EventID: eventID, SourceID: record.SourceID,
		SessionID: record.SessionID, TurnID: record.TurnID, Signal: record.Signal, Kind: kind,
		Timestamp: formatApplicationTimestamp(timestamp), StartAt: formatApplicationTimestamp(record.StartTimeNS),
		EndAt: formatApplicationTimestamp(record.EndTimeNS), SpanID: optionalAttributeString(record.SpanID),
		ParentSpanID: optionalAttributeString(record.ParentSpanID), SpanRole: optionalAttributeString(record.SpanRole),
		DataQuality: record.DataQuality, CorrelationMethod: optionalAttributeString(record.CorrelationMethod),
		Tool: toolName, ToolCallID: record.ToolCallID, Model: model, Outcome: outcome,
		RetryIndex: record.RetryIndex, ProcessPID: record.ProcessPID, ParentProcessPID: record.ParentProcessPID,
		DurationMS: duration, TTFTMS: attributeValue(record.Attributes, "ttft_ms"),
		InputTokens:  attributeValue(record.Attributes, "input_token_count", "gen_ai.usage.input_tokens"),
		OutputTokens: attributeValue(record.Attributes, "output_token_count", "gen_ai.usage.output_tokens"),
	}
}

func eventKind(eventName string, attributes map[string]any) string {
	if attributeString(attributes, "tool_name", "tool.name") != "" {
		return "tool_call"
	}
	if attributeString(attributes, "model", "gen_ai.response.model", "gen_ai.request.model") != "" {
		return "model_event"
	}
	if eventName != "" {
		return eventName
	}
	return "telemetry"
}

func attributeString(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := attributes[key].(type) {
		case string:
			return value
		case bool:
			return fmt.Sprint(value)
		}
	}
	return ""
}

func attributeValue(attributes map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := attributes[key]; exists {
			return value
		}
	}
	return nil
}

func optionalAttributeString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func formatApplicationTimestamp(timestamp *int64) *string {
	if timestamp == nil || *timestamp <= 0 {
		return nil
	}
	formatted := time.Unix(0, *timestamp).UTC().Format(time.RFC3339Nano)
	return &formatted
}

func (s *SessionQueryService) ToolSummary(ctx context.Context, rangeName, sessionID string) ([]telemetry.ToolSummary, error) {
	duration, okay := SummaryRangeDuration(rangeName)
	if !okay {
		return nil, ErrInvalidSessionQueryRange
	}
	untilNS := s.now().UnixNano()
	return s.repository.ListToolSummary(ctx, untilNS-duration.Nanoseconds(), untilNS, sessionID)
}

func (s *SessionQueryService) ToolCalls(ctx context.Context, query ToolCallQuery) ([]telemetry.ToolCallRecord, error) {
	duration, okay := SummaryRangeDuration(query.Range)
	if !okay {
		return nil, ErrInvalidSessionQueryRange
	}
	if query.Outcome != "" && query.Outcome != "successful" && query.Outcome != "failed" && query.Outcome != "unknown" {
		return nil, ErrInvalidToolOutcome
	}
	untilNS := s.now().UnixNano()
	return s.repository.ListToolCalls(ctx, untilNS-duration.Nanoseconds(), untilNS,
		query.SessionID, query.ToolName, query.Outcome, query.Limit, query.BeforeAtNS, query.BeforeID)
}

func SummaryRangeDuration(value string) (time.Duration, bool) {
	switch value {
	case "15m":
		return 15 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}
