package correlation

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

func TestMergeTraceSpanInheritsExactObservedIdentifiers(t *testing.T) {
	event := telemetry.CanonicalMetadata{SourceID: telemetry.SourceCodex, CorrelationMethod: "unassigned", DataQuality: "partial"}
	span := telemetry.CanonicalMetadata{
		SourceID: telemetry.SourceCodex, ExternalSessionID: "session-1", SessionID: "codex:session-1",
		ExternalTurnID: "turn-1", TurnID: "codex:turn-1", CorrelationMethod: "observed_identifier",
	}
	merged, inherited := MergeTraceSpan(event, span)
	if !inherited || merged.SessionID != span.SessionID || merged.TurnID != span.TurnID || merged.CorrelationMethod != "trace_span" {
		t.Fatalf("merged association = %+v, inherited=%t", merged, inherited)
	}
	if merged.CorrelationConfidence == nil || *merged.CorrelationConfidence != 1 {
		t.Fatalf("exact trace/span association confidence = %v, want 1", merged.CorrelationConfidence)
	}
	if merged.DataQuality != "partial" {
		t.Fatalf("association rule changed timestamp quality to %q", merged.DataQuality)
	}
}

func TestMergeTraceSpanPreservesConflictsAndSourceBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		event telemetry.CanonicalMetadata
		span  telemetry.CanonicalMetadata
	}{
		{
			name:  "session conflict",
			event: telemetry.CanonicalMetadata{SourceID: telemetry.SourceCodex, ExternalSessionID: "event-session", SessionID: "codex:event-session"},
			span:  telemetry.CanonicalMetadata{SourceID: telemetry.SourceCodex, ExternalSessionID: "span-session", SessionID: "codex:span-session"},
		},
		{
			name:  "turn conflict",
			event: telemetry.CanonicalMetadata{SourceID: telemetry.SourceCodex, ExternalTurnID: "event-turn", TurnID: "codex:event-turn"},
			span:  telemetry.CanonicalMetadata{SourceID: telemetry.SourceCodex, ExternalTurnID: "span-turn", TurnID: "codex:span-turn"},
		},
		{
			name:  "source conflict",
			event: telemetry.CanonicalMetadata{SourceID: "codex"},
			span:  telemetry.CanonicalMetadata{SourceID: "other-agent", ExternalSessionID: "session-1", SessionID: "other-agent:session-1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merged, inherited := MergeTraceSpan(test.event, test.span)
			if inherited || merged.SessionID != test.event.SessionID || merged.TurnID != test.event.TurnID {
				t.Fatalf("conflicting association was changed: %+v, inherited=%t", merged, inherited)
			}
		})
	}
}
