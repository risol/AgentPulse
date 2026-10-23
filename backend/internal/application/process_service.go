// Package application coordinates process monitoring use cases between the
// process domain, persistence, correlation rules, and the UI/API boundary.
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentpulse/agentpulse/backend/internal/correlation"
	"github.com/agentpulse/agentpulse/backend/internal/process"
)

var (
	ErrInvalidProcessSettings     = errors.New("invalid process monitor settings")
	ErrProcessSettingsPersistence = errors.New("persist process monitor settings")
	ErrProcessMonitorUpdate       = errors.New("apply process monitor settings")
)

type ProcessRepository interface {
	LoadProcessMonitorConfig(context.Context) (process.Config, error)
	SaveProcessMonitorConfig(context.Context, process.Config) error
	ListProcesses(context.Context, int, *int64, string) ([]process.Record, error)
	ListProcessesForSession(context.Context, string) ([]process.Record, error)
	ListProcessToolSpans(context.Context, int64, int64) ([]correlation.ToolSpan, error)
	ListProcessToolSpansForSession(context.Context, string) ([]correlation.ToolSpan, error)
}

type ProcessMonitor interface {
	Configure(process.Config) error
	Status() process.MonitorStatus
}

type ProcessView struct {
	ProcessID             string   `json:"process_id"`
	PID                   uint32   `json:"pid"`
	ParentPID             uint32   `json:"parent_pid"`
	ImageName             string   `json:"image_name"`
	ExecutablePath        string   `json:"executable_path,omitempty"`
	CommandLine           string   `json:"command_line,omitempty"`
	StartedAt             *string  `json:"started_at"`
	EndedAt               *string  `json:"ended_at"`
	ExitCode              *uint32  `json:"exit_code"`
	LastSampleAt          *string  `json:"last_sample_at"`
	CPUTimeMS             *float64 `json:"cpu_time_ms"`
	WorkingSetBytes       *uint64  `json:"working_set_bytes"`
	SampleQuality         string   `json:"sample_quality"`
	CommandCategory       *string  `json:"command_category"`
	CategoryMethod        *string  `json:"category_method"`
	AssociationMethod     string   `json:"association_method"`
	AssociationConfidence *float64 `json:"association_confidence"`
	AssociatedToolName    *string  `json:"associated_tool_name"`
	AssociatedToolCallID  *string  `json:"associated_tool_call_id"`
	AssociatedSpanID      *string  `json:"associated_span_id"`

	OrderAtNS int64 `json:"-"`
}

type ProcessService struct {
	repository ProcessRepository
	monitor    ProcessMonitor
}

func NewProcessService(repository ProcessRepository, monitor ProcessMonitor) *ProcessService {
	return &ProcessService{repository: repository, monitor: monitor}
}

func (s *ProcessService) LoadSettings(ctx context.Context) (process.Config, error) {
	return s.repository.LoadProcessMonitorConfig(ctx)
}

func (s *ProcessService) UpdateSettings(ctx context.Context, config process.Config) (process.Config, error) {
	validated, err := process.ValidateConfig(config)
	if err != nil {
		return process.Config{}, fmt.Errorf("%w: %v", ErrInvalidProcessSettings, err)
	}
	if err := s.repository.SaveProcessMonitorConfig(ctx, validated); err != nil {
		return process.Config{}, fmt.Errorf("%w: %v", ErrProcessSettingsPersistence, err)
	}
	if s.monitor != nil {
		if err := s.monitor.Configure(validated); err != nil {
			return process.Config{}, fmt.Errorf("%w: %v", ErrProcessMonitorUpdate, err)
		}
	}
	return validated, nil
}

func (s *ProcessService) Status() process.MonitorStatus {
	if s.monitor == nil {
		return process.MonitorStatus{Enabled: false, State: "disabled"}
	}
	return s.monitor.Status()
}

func (s *ProcessService) List(ctx context.Context, limit int, beforeAtNS *int64, beforeID string) ([]ProcessView, error) {
	config, err := s.repository.LoadProcessMonitorConfig(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListProcesses(ctx, limit, beforeAtNS, beforeID)
	if err != nil {
		return nil, err
	}
	views := make([]ProcessView, 0, len(records))
	if len(records) == 0 {
		return views, nil
	}

	fromNS, toNS := records[0].StartedAtNS, processEndAt(records[0])
	for _, record := range records[1:] {
		if record.StartedAtNS < fromNS {
			fromNS = record.StartedAtNS
		}
		if endAt := processEndAt(record); endAt > toNS {
			toNS = endAt
		}
	}
	spans, err := s.repository.ListProcessToolSpans(ctx, fromNS, toNS)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		views = append(views, mapProcessView(record, config, spans))
	}
	return views, nil
}

// ListForSession returns only processes that can be linked to a tool span in
// the requested session. The same explicit PID and confidence-labelled
// fallback rules used by the process explorer are applied here.
func (s *ProcessService) ListForSession(ctx context.Context, sessionID string) ([]ProcessView, error) {
	config, err := s.repository.LoadProcessMonitorConfig(ctx)
	if err != nil {
		return nil, err
	}
	spans, err := s.repository.ListProcessToolSpansForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return []ProcessView{}, nil
	}
	records, err := s.repository.ListProcessesForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	views := make([]ProcessView, 0, len(records))
	for _, record := range records {
		view := mapProcessView(record, config, spans)
		if view.AssociationMethod != "unassigned" {
			views = append(views, view)
		}
	}
	return views, nil
}

func mapProcessView(record process.Record, config process.Config, spans []correlation.ToolSpan) ProcessView {
	category, matched := process.ClassifyImageName(record.ImageName, config.CategoryRules)
	var categoryValue, categoryMethod *string
	if matched {
		categoryValue = stringPointer(category)
		method := "configured_image_name_rule"
		categoryMethod = &method
	}
	association := correlation.AssociateProcess(record, category, spans)
	startedAt := record.StartedAtNS
	return ProcessView{
		ProcessID: record.ProcessID, PID: record.PID, ParentPID: record.ParentPID,
		ImageName: record.ImageName, ExecutablePath: record.ExecutablePath, CommandLine: record.CommandLine,
		StartedAt: formatApplicationTimestamp(&startedAt),
		EndedAt:   formatApplicationTimestamp(record.EndedAtNS), ExitCode: record.ExitCode,
		LastSampleAt: formatApplicationTimestamp(record.LastSampleAtNS), CPUTimeMS: record.CPUTimeMS,
		WorkingSetBytes: record.WorkingSetBytes, SampleQuality: record.SampleQuality,
		CommandCategory: categoryValue, CategoryMethod: categoryMethod,
		AssociationMethod: association.Method, AssociationConfidence: association.Confidence,
		AssociatedToolName:   optionalApplicationString(association.ToolName),
		AssociatedToolCallID: optionalApplicationString(association.ToolCallID),
		AssociatedSpanID:     optionalApplicationString(association.SpanID),
		OrderAtNS:            record.OrderAtNS,
	}
}

func processEndAt(record process.Record) int64 {
	if record.EndedAtNS != nil {
		return *record.EndedAtNS
	}
	if record.LastSampleAtNS != nil {
		return *record.LastSampleAtNS
	}
	return record.StartedAtNS
}

func stringPointer(value string) *string { return &value }

func optionalApplicationString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
