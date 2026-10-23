// Package codexadapter maps allowlisted Codex OTLP attributes to canonical
// session, turn, and tool-call metadata before storage receives a batch.
package codexadapter

import (
	"context"
	"errors"
	"strings"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

var ErrUnsupportedSource = errors.New("Codex adapter received an unsupported source")

type Repository interface {
	SaveTelemetry(context.Context, telemetry.Batch) error
}

type Writer struct{ repository Repository }

func NewWriter(repository Repository) *Writer { return &Writer{repository: repository} }

func (w *Writer) SaveTelemetry(ctx context.Context, batch telemetry.Batch) error {
	normalized, err := Normalize(batch)
	if err != nil {
		return err
	}
	return w.repository.SaveTelemetry(ctx, normalized)
}

func Normalize(batch telemetry.Batch) (telemetry.Batch, error) {
	sourceID := strings.TrimSpace(batch.SourceID)
	if sourceID == "" {
		sourceID = telemetry.SourceCodex
	}
	if sourceID != telemetry.SourceCodex {
		return telemetry.Batch{}, ErrUnsupportedSource
	}
	batch.SourceID = sourceID
	batch.EventMetadata = make([]telemetry.CanonicalMetadata, len(batch.Events))
	for index, event := range batch.Events {
		batch.EventMetadata[index] = normalizeEvent(batch.Signal, sourceID, batch.ReceivedAtNS, event)
	}
	batch.SpanMetadata = make([]telemetry.CanonicalMetadata, len(batch.Spans))
	for index, span := range batch.Spans {
		batch.SpanMetadata[index] = normalizeSpan(sourceID, span)
	}
	return batch, nil
}

func normalizeEvent(signal telemetry.Signal, sourceID string, receivedAtNS int64, event telemetry.Event) telemetry.CanonicalMetadata {
	attributes := event.Attributes
	metadata := baseMetadata(sourceID, attributes)
	metadata.Model = firstString(attributes, "model", "gen_ai.response.model", "gen_ai.request.model")
	metadata.ActivityAtNS = event.EventTimeNS
	if metadata.ActivityAtNS == nil && receivedAtNS > 0 {
		metadata.ActivityAtNS = int64Pointer(receivedAtNS)
	}
	if event.EventTimeNS != nil && (metadata.SessionID != "" || metadata.TurnID != "") {
		metadata.DataQuality = "complete"
	}
	if signal == telemetry.SignalLogs {
		metadata.InputTokens = tokenAttribute(attributes, "input_token_count", "gen_ai.usage.input_tokens")
		metadata.OutputTokens = tokenAttribute(attributes, "output_token_count", "gen_ai.usage.output_tokens")
		metadata.ReasoningTokens = tokenAttribute(attributes, "reasoning_token_count", "codex.usage.reasoning_output_tokens")
		metadata.TTFTMS = floatAttribute(attributes, "ttft_ms")
	}
	if toolName := firstString(attributes, "tool.name", "tool_name"); toolName != "" {
		metadata.ToolCall = &telemetry.ToolCallObservation{
			ExternalID:  firstString(attributes, "tool.call_id", "call_id"),
			Name:        toolName,
			Type:        firstString(attributes, "tool.type"),
			StartTimeNS: event.EventTimeNS,
			DurationMS:  floatAttribute(attributes, "duration_ms"),
			Status:      statusAttribute(attributes),
			RetryIndex:  integerAttribute(attributes, "retry_index"),
			TraceID:     event.TraceID,
			SpanID:      event.SpanID,
		}
	}
	return metadata
}

func normalizeSpan(sourceID string, span telemetry.Span) telemetry.CanonicalMetadata {
	metadata := baseMetadata(sourceID, span.Attributes)
	metadata.Model = firstString(span.Attributes, "model", "gen_ai.response.model", "gen_ai.request.model")
	metadata.SpanRole = "other"
	metadata.ProcessPID = intAttribute(span.Attributes, "process.pid")
	metadata.ParentProcessPID = intAttribute(span.Attributes, "process.parent_pid")
	if firstString(span.Attributes, "tool.name", "tool_name") != "" {
		metadata.SpanRole = "tool"
	} else if metadata.Model != "" || firstString(span.Attributes, "gen_ai.operation.name") != "" {
		metadata.SpanRole = "model"
	}
	if span.StartTimeNS > 0 {
		metadata.ActivityAtNS = int64Pointer(span.StartTimeNS)
	}
	if metadata.ActivityAtNS != nil && (metadata.SessionID != "" || metadata.TurnID != "") {
		metadata.DataQuality = "complete"
	}
	if toolName := firstString(span.Attributes, "tool.name", "tool_name"); toolName != "" {
		var endAt *int64
		var duration *float64
		if span.EndTimeNS > span.StartTimeNS && span.StartTimeNS > 0 {
			endAt = int64Pointer(span.EndTimeNS)
			value := float64(span.EndTimeNS-span.StartTimeNS) / 1e6
			duration = &value
		}
		if duration == nil {
			duration = floatAttribute(span.Attributes, "duration_ms")
		}
		status := statusAttribute(span.Attributes)
		if span.StatusCode == 1 {
			status = "success"
		} else if span.StatusCode == 2 {
			status = "error"
		}
		metadata.ToolCall = &telemetry.ToolCallObservation{
			ExternalID:  firstString(span.Attributes, "tool.call_id", "call_id"),
			Name:        toolName,
			Type:        firstString(span.Attributes, "tool.type"),
			StartTimeNS: metadata.ActivityAtNS,
			EndTimeNS:   endAt,
			DurationMS:  duration,
			Status:      status,
			RetryIndex:  integerAttribute(span.Attributes, "retry_index"),
			TraceID:     span.TraceID,
			SpanID:      span.SpanID,
		}
	}
	return metadata
}

func baseMetadata(sourceID string, attributes map[string]any) telemetry.CanonicalMetadata {
	metadata := telemetry.CanonicalMetadata{
		SourceID: sourceID, CorrelationMethod: "unassigned", DataQuality: "partial",
		ExternalSessionID: firstString(attributes, "conversation.id", "session.id"),
		ExternalTurnID:    firstString(attributes, "turn.id"),
	}
	metadata.SessionID = canonicalID(sourceID, metadata.ExternalSessionID)
	metadata.TurnID = canonicalID(sourceID, metadata.ExternalTurnID)
	if metadata.SessionID != "" {
		metadata.CorrelationMethod = "observed_identifier"
		metadata.CorrelationConfidence = floatPointer(1)
	} else if metadata.TurnID != "" {
		metadata.CorrelationMethod = "observed_turn_identifier"
		metadata.CorrelationConfidence = floatPointer(1)
	}
	return metadata
}

func firstString(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, okay := attributes[key].(string); okay {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func tokenAttribute(attributes map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		switch value := attributes[key].(type) {
		case int:
			if value >= 0 {
				return int64Pointer(int64(value))
			}
		case int64:
			if value >= 0 {
				return int64Pointer(value)
			}
		case uint64:
			if value <= uint64(^uint64(0)>>1) {
				return int64Pointer(int64(value))
			}
		}
	}
	return nil
}

func floatAttribute(attributes map[string]any, key string) *float64 {
	var value float64
	switch typed := attributes[key].(type) {
	case float64:
		value = typed
	case float32:
		value = float64(typed)
	case int:
		value = float64(typed)
	case int64:
		value = float64(typed)
	default:
		return nil
	}
	if value < 0 {
		return nil
	}
	return &value
}

func intAttribute(attributes map[string]any, key string) *int64 {
	switch value := attributes[key].(type) {
	case int:
		if value > 0 {
			return int64Pointer(int64(value))
		}
	case int64:
		if value > 0 {
			return int64Pointer(value)
		}
	case uint64:
		if value > 0 && value <= uint64(^uint64(0)>>1) {
			return int64Pointer(int64(value))
		}
	}
	return nil
}

func integerAttribute(attributes map[string]any, key string) *int64 {
	switch value := attributes[key].(type) {
	case int:
		if value >= 0 {
			return int64Pointer(int64(value))
		}
	case int64:
		if value >= 0 {
			return int64Pointer(value)
		}
	case uint64:
		if value <= uint64(^uint64(0)>>1) {
			return int64Pointer(int64(value))
		}
	}
	return nil
}

func statusAttribute(attributes map[string]any) string {
	if success, okay := attributes["success"].(bool); okay {
		if success {
			return "success"
		}
		return "error"
	}
	return firstString(attributes, "outcome", "error.type")
}

func canonicalID(sourceID, externalID string) string {
	if externalID == "" {
		return ""
	}
	return sourceID + ":" + externalID
}

func int64Pointer(value int64) *int64     { return &value }
func floatPointer(value float64) *float64 { return &value }

var _ Repository = (*Writer)(nil)
