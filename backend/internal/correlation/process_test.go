package correlation

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/process"
)

func TestAssociateProcessPrefersExplicitProcessIdentity(t *testing.T) {
	pid := uint32(44)
	record := process.Record{PID: pid, StartedAtNS: 100, EndedAtNS: int64Pointer(300)}
	association := AssociateProcess(record, "shell", []ToolSpan{
		{SpanID: "span-direct", ToolCallID: "call-1", ToolName: "shell", StartTimeNS: 120, EndTimeNS: 250, ProcessPID: &pid},
		{SpanID: "span-overlap", ToolCallID: "call-2", ToolName: "shell", StartTimeNS: 120, EndTimeNS: 250},
	})
	if association.Method != "explicit_process_pid" || association.Confidence == nil || *association.Confidence != 1 || association.ToolCallID != "call-1" {
		t.Fatalf("explicit identity did not win: %+v", association)
	}
}

func TestAssociateProcessUsesParentAndTimeWithLowerConfidence(t *testing.T) {
	parentPID := uint32(10)
	record := process.Record{PID: 44, ParentPID: parentPID, StartedAtNS: 100, EndedAtNS: int64Pointer(300)}
	association := AssociateProcess(record, "shell", []ToolSpan{
		{SpanID: "span-1", ToolCallID: "call-1", ToolName: "exec_command", StartTimeNS: 90, EndTimeNS: 310, ProcessPID: &parentPID},
	})
	if association.Method != "parent_pid_time_overlap" || association.Confidence == nil || *association.Confidence != 0.75 {
		t.Fatalf("parent/time association = %+v", association)
	}
}

func TestAssociateProcessPrefersSpecificCommandOverExecEnvelope(t *testing.T) {
	record := process.Record{PID: 44, StartedAtNS: 100, EndedAtNS: int64Pointer(300)}
	association := AssociateProcess(record, "shell", []ToolSpan{
		{SpanID: "outer", ToolCallID: "outer-call", ToolName: "exec", StartTimeNS: 90, EndTimeNS: 310},
		{SpanID: "command", ToolCallID: "command-call", ToolName: "exec_command", StartTimeNS: 100, EndTimeNS: 300},
	})
	if association.Method != "time_and_category_unique_candidate" || association.ToolCallID != "command-call" || association.ToolName != "exec_command" {
		t.Fatalf("specific command span did not win over its generic envelope: %+v", association)
	}
}

func TestAssociateProcessOnlyUsesUniqueCommandCategoryCandidates(t *testing.T) {
	record := process.Record{PID: 44, StartedAtNS: 100, EndedAtNS: int64Pointer(300)}
	spans := []ToolSpan{{SpanID: "span-1", ToolCallID: "call-1", ToolName: "shell", StartTimeNS: 150, EndTimeNS: 250}}
	association := AssociateProcess(record, "shell", spans)
	if association.Method != "time_and_category_unique_candidate" || association.Confidence == nil || *association.Confidence != 0.55 {
		t.Fatalf("unique time/category association = %+v", association)
	}

	spans = append(spans, ToolSpan{SpanID: "span-2", ToolCallID: "call-2", ToolName: "shell_command", StartTimeNS: 160, EndTimeNS: 240})
	association = AssociateProcess(record, "shell", spans)
	if association.Method != "unassigned" || association.Confidence != nil {
		t.Fatalf("ambiguous association should remain unassigned: %+v", association)
	}
	association = AssociateProcess(record, "build", spans[:1])
	if association.Method != "unassigned" || association.Confidence != nil {
		t.Fatalf("non-shell process should not use shell timing heuristic: %+v", association)
	}
}

func TestAssociateProcessRejectsMissingOrNonOverlappingIntervals(t *testing.T) {
	pid := uint32(44)
	record := process.Record{PID: pid, StartedAtNS: 100, EndedAtNS: int64Pointer(100)}
	association := AssociateProcess(record, "shell", []ToolSpan{{
		SpanID: "span", ToolCallID: "call", ToolName: "shell", StartTimeNS: 90, EndTimeNS: 120, ProcessPID: &pid,
	}})
	if association.Method != "unassigned" || association.Confidence != nil {
		t.Fatalf("zero-duration process should remain unassigned: %+v", association)
	}
}

func int64Pointer(value int64) *int64 { return &value }
