package codexadapter

import (
	"errors"
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

func TestNormalizeMapsObservedCodexIdentifiersAndMetrics(t *testing.T) {
	eventTime := int64(1700000000000000000)
	batch, err := Normalize(telemetry.Batch{
		Signal: telemetry.SignalLogs, ReceivedAtNS: eventTime + 5,
		Events: []telemetry.Event{{
			EventTimeNS: &eventTime, TraceID: "trace", SpanID: "event-span", Name: "log_record",
			Attributes: map[string]any{
				"conversation.id": " conversation-7 ", "turn.id": "turn-3", "model": "gpt-test",
				"input_token_count": int64(12), "output_token_count": int64(4),
				"tool_name": "shell", "tool.call_id": "call-1", "success": true, "ttft_ms": float64(27.5),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.SourceID != telemetry.SourceCodex || len(batch.EventMetadata) != 1 {
		t.Fatalf("normalized batch identity = %q with %d event metadata entries", batch.SourceID, len(batch.EventMetadata))
	}
	metadata := batch.EventMetadata[0]
	if metadata.ExternalSessionID != "conversation-7" || metadata.SessionID != "codex:conversation-7" || metadata.ExternalTurnID != "turn-3" || metadata.TurnID != "codex:turn-3" {
		t.Fatalf("observed identifiers were mapped incorrectly: %+v", metadata)
	}
	if metadata.CorrelationMethod != "observed_identifier" || metadata.DataQuality != "complete" || metadata.Model != "gpt-test" {
		t.Fatalf("association and quality were mapped incorrectly: %+v", metadata)
	}
	if metadata.InputTokens == nil || *metadata.InputTokens != 12 || metadata.OutputTokens == nil || *metadata.OutputTokens != 4 || metadata.ReasoningTokens != nil {
		t.Fatalf("token fields lost missing-vs-zero semantics: %+v", metadata)
	}
	if metadata.TTFTMS == nil || *metadata.TTFTMS != 27.5 {
		t.Fatalf("TTFT was not normalized: %+v", metadata.TTFTMS)
	}
	if metadata.ToolCall == nil || metadata.ToolCall.Name != "shell" || metadata.ToolCall.ExternalID != "call-1" || metadata.ToolCall.Status != "success" || metadata.ToolCall.StartTimeNS == nil || *metadata.ToolCall.StartTimeNS != eventTime {
		t.Fatalf("tool call metadata was mapped incorrectly: %+v", metadata.ToolCall)
	}
}

func TestNormalizeClassifiesSpanRolesAndRejectsUnknownSources(t *testing.T) {
	batch, err := Normalize(telemetry.Batch{
		Signal: telemetry.SignalTraces,
		Spans: []telemetry.Span{
			{TraceID: "trace", SpanID: "tool", StartTimeNS: 100, EndTimeNS: 200, StatusCode: 2, Attributes: map[string]any{"tool.name": "shell", "conversation.id": "session", "process.pid": int64(42), "process.parent_pid": int64(12)}},
			{TraceID: "trace", SpanID: "model", StartTimeNS: 100, EndTimeNS: 300, Attributes: map[string]any{"gen_ai.operation.name": "chat", "gen_ai.response.model": "gpt-test"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.SpanMetadata) != 2 {
		t.Fatalf("span metadata count = %d, want 2", len(batch.SpanMetadata))
	}
	tool := batch.SpanMetadata[0]
	if tool.SpanRole != "tool" || tool.ToolCall == nil || tool.ToolCall.Status != "error" || tool.ToolCall.DurationMS == nil || *tool.ToolCall.DurationMS != 0.0001 || tool.ProcessPID == nil || *tool.ProcessPID != 42 || tool.ParentProcessPID == nil || *tool.ParentProcessPID != 12 {
		t.Fatalf("tool span was not normalized: %+v", tool)
	}
	model := batch.SpanMetadata[1]
	if model.SpanRole != "model" || model.Model != "gpt-test" || model.ToolCall != nil {
		t.Fatalf("model span was not normalized: %+v", model)
	}
	if _, err := Normalize(telemetry.Batch{SourceID: "other-agent"}); !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("unsupported source returned %v, want ErrUnsupportedSource", err)
	}
}
