package correlation

import "github.com/agentpulse/agentpulse/backend/internal/telemetry"

// MergeTraceSpan inherits identifiers only when a telemetry event and span
// have an exact trace/span relationship and their observed IDs do not conflict.
// It leaves timestamp quality decisions to the caller because receipt time is
// not equivalent to an observed event timestamp.
func MergeTraceSpan(event, span telemetry.CanonicalMetadata) (telemetry.CanonicalMetadata, bool) {
	if event.SourceID != "" && span.SourceID != "" && event.SourceID != span.SourceID {
		return event, false
	}
	if conflicts(event.ExternalSessionID, span.ExternalSessionID) || conflicts(event.SessionID, span.SessionID) ||
		conflicts(event.ExternalTurnID, span.ExternalTurnID) || conflicts(event.TurnID, span.TurnID) {
		return event, false
	}
	merged := event
	inherited := false
	if merged.ExternalSessionID == "" && span.ExternalSessionID != "" {
		merged.ExternalSessionID = span.ExternalSessionID
		inherited = true
	}
	if merged.SessionID == "" && span.SessionID != "" {
		merged.SessionID = span.SessionID
		inherited = true
	}
	if merged.ExternalTurnID == "" && span.ExternalTurnID != "" {
		merged.ExternalTurnID = span.ExternalTurnID
		inherited = true
	}
	if merged.TurnID == "" && span.TurnID != "" {
		merged.TurnID = span.TurnID
		inherited = true
	}
	if inherited {
		merged.CorrelationMethod = "trace_span"
		confidence := 1.0
		merged.CorrelationConfidence = &confidence
	}
	return merged, inherited
}

func conflicts(first, second string) bool {
	return first != "" && second != "" && first != second
}
