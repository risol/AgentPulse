// Package correlation contains explicit, confidence-labelled links between
// independently observed process records and tool spans.
package correlation

import (
	"strings"

	"github.com/agentpulse/agentpulse/backend/internal/process"
)

type ToolSpan struct {
	SpanID           string
	SessionID        string
	ToolCallID       string
	ToolName         string
	StartTimeNS      int64
	EndTimeNS        int64
	ProcessPID       *uint32
	ParentProcessPID *uint32
}

type Association struct {
	Method     string
	Confidence *float64
	SpanID     string
	ToolCallID string
	ToolName   string
}

func AssociateProcess(record process.Record, category string, spans []ToolSpan) Association {
	endAt := record.StartedAtNS
	if record.EndedAtNS != nil {
		endAt = *record.EndedAtNS
	} else if record.LastSampleAtNS != nil {
		endAt = *record.LastSampleAtNS
	}
	if endAt <= record.StartedAtNS {
		return Association{Method: "unassigned"}
	}

	direct := make([]ToolSpan, 0, 1)
	for _, span := range spans {
		if span.ProcessPID != nil && *span.ProcessPID == record.PID && overlaps(record.StartedAtNS, endAt, span.StartTimeNS, span.EndTimeNS) {
			direct = append(direct, span)
		}
	}
	if candidate, okay := uniqueBestCandidate(direct, record.StartedAtNS, endAt); okay {
		return association(candidate, "explicit_process_pid", 1)
	}
	if len(direct) > 0 {
		return Association{Method: "unassigned"}
	}

	if category != "shell" {
		return Association{Method: "unassigned"}
	}
	parentMatches := make([]ToolSpan, 0, 1)
	for _, span := range spans {
		if span.ProcessPID != nil && *span.ProcessPID == record.ParentPID && IsCommandTool(span.ToolName) && overlaps(record.StartedAtNS, endAt, span.StartTimeNS, span.EndTimeNS) {
			parentMatches = append(parentMatches, span)
		}
	}
	parentMatches = preferSpecificCommandTools(parentMatches, record.StartedAtNS, endAt)
	if candidate, okay := uniqueBestCandidate(parentMatches, record.StartedAtNS, endAt); okay {
		return association(candidate, "parent_pid_time_overlap", 0.75)
	}
	if len(parentMatches) > 0 {
		return Association{Method: "unassigned"}
	}

	timeMatches := make([]ToolSpan, 0, 1)
	for _, span := range spans {
		if IsCommandTool(span.ToolName) && overlaps(record.StartedAtNS, endAt, span.StartTimeNS, span.EndTimeNS) {
			timeMatches = append(timeMatches, span)
		}
	}
	timeMatches = preferSpecificCommandTools(timeMatches, record.StartedAtNS, endAt)
	if candidate, okay := uniqueBestCandidate(timeMatches, record.StartedAtNS, endAt); okay {
		return association(candidate, "time_and_category_unique_candidate", 0.55)
	}
	return Association{Method: "unassigned"}
}

// Codex can emit an outer "exec" span around the more specific "exec_command"
// span for one shell operation. Ignore that generic envelope only when the
// remaining, more specific command spans identify one unique call. Multiple
// specific candidates stay unassigned.
func preferSpecificCommandTools(spans []ToolSpan, startAt, endAt int64) []ToolSpan {
	specific := make([]ToolSpan, 0, len(spans))
	for _, span := range spans {
		if strings.ToLower(strings.TrimSpace(span.ToolName)) != "exec" {
			specific = append(specific, span)
		}
	}
	if len(specific) == 0 {
		return spans
	}
	if _, unique := uniqueBestCandidate(specific, startAt, endAt); unique {
		return specific
	}
	return spans
}

func association(span ToolSpan, method string, confidence float64) Association {
	return Association{
		Method: method, Confidence: &confidence, SpanID: span.SpanID,
		ToolCallID: span.ToolCallID, ToolName: span.ToolName,
	}
}

func uniqueBestCandidate(spans []ToolSpan, startAt, endAt int64) (ToolSpan, bool) {
	if len(spans) == 0 {
		return ToolSpan{}, false
	}
	bestByCall := make(map[string]ToolSpan, len(spans))
	for _, span := range spans {
		key := span.ToolCallID
		if key == "" {
			key = "span:" + span.SpanID
		}
		previous, exists := bestByCall[key]
		if !exists || overlapDuration(startAt, endAt, span.StartTimeNS, span.EndTimeNS) > overlapDuration(startAt, endAt, previous.StartTimeNS, previous.EndTimeNS) {
			bestByCall[key] = span
		}
	}
	if len(bestByCall) != 1 {
		return ToolSpan{}, false
	}
	for _, span := range bestByCall {
		return span, true
	}
	return ToolSpan{}, false
}

func overlaps(firstStart, firstEnd, secondStart, secondEnd int64) bool {
	return firstEnd > firstStart && secondEnd > secondStart && firstStart < secondEnd && secondStart < firstEnd
}

func overlapDuration(firstStart, firstEnd, secondStart, secondEnd int64) int64 {
	start := firstStart
	if secondStart > start {
		start = secondStart
	}
	end := firstEnd
	if secondEnd < end {
		end = secondEnd
	}
	if end <= start {
		return 0
	}
	return end - start
}

func IsCommandTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "shell", "shell_command", "exec", "exec_command", "command", "run_command", "terminal":
		return true
	default:
		return false
	}
}
