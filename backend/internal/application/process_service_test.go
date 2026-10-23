package application

import (
	"context"
	"errors"
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/correlation"
	"github.com/agentpulse/agentpulse/backend/internal/process"
)

func TestUpdateSettingsPersistsValidatedConfigBeforeApplyingIt(t *testing.T) {
	repository := &fakeProcessRepository{config: process.DefaultConfig()}
	monitor := &fakeProcessMonitor{}
	service := NewProcessService(repository, monitor)
	config := process.DefaultConfig()
	config.Enabled = true
	config.RootProcessNames = []string{"Codex"}

	updated, err := service.UpdateSettings(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.RootProcessNames[0] != "codex.exe" || !repository.saved || !monitor.configured || monitor.config.RootProcessNames[0] != "codex.exe" {
		t.Fatalf("settings were not persisted and applied: updated=%+v repository=%+v monitor=%+v", updated, repository, monitor)
	}
}

func TestUpdateSettingsDoesNotPersistInvalidConfig(t *testing.T) {
	repository := &fakeProcessRepository{config: process.DefaultConfig()}
	service := NewProcessService(repository, &fakeProcessMonitor{})
	config := process.DefaultConfig()
	config.SampleIntervalMS = 1

	_, err := service.UpdateSettings(context.Background(), config)
	if !errors.Is(err, ErrInvalidProcessSettings) || repository.saved {
		t.Fatalf("invalid settings handling: error=%v saved=%t", err, repository.saved)
	}
}

func TestListBuildsCategoryAndConfidenceLabelledAssociation(t *testing.T) {
	pid := uint32(44)
	repository := &fakeProcessRepository{
		config:  process.DefaultConfig(),
		records: []process.Record{{ProcessID: "process-44@100", PID: pid, ImageName: "pwsh.exe", ExecutablePath: "%USERPROFILE%\\tools\\pwsh.exe", CommandLine: "pwsh -File run.ps1 --api-key=[redacted]", StartedAtNS: 100, OrderAtNS: 300, EndedAtNS: int64Pointer(300)}},
		spans:   []correlation.ToolSpan{{SpanID: "span", ToolCallID: "call", ToolName: "shell", StartTimeNS: 120, EndTimeNS: 280, ProcessPID: &pid}},
	}
	service := NewProcessService(repository, nil)
	views, err := service.List(context.Background(), 10, nil, "")
	if err != nil || len(views) != 1 {
		t.Fatalf("process views = %+v, error = %v", views, err)
	}
	view := views[0]
	if view.CommandCategory == nil || *view.CommandCategory != "shell" || view.CategoryMethod == nil || *view.CategoryMethod != "configured_image_name_rule" {
		t.Fatalf("process category was not mapped: %+v", view)
	}
	if view.AssociationMethod != "explicit_process_pid" || view.AssociationConfidence == nil || *view.AssociationConfidence != 1 || view.AssociatedToolCallID == nil || *view.AssociatedToolCallID != "call" {
		t.Fatalf("process association was not mapped: %+v", view)
	}
	if view.ProcessID != "process-44@100" || view.PID != pid || view.StartedAt == nil || view.EndedAt == nil || view.OrderAtNS != 300 {
		t.Fatalf("process response view is missing mapped fields: %+v", view)
	}
	if view.ExecutablePath != "%USERPROFILE%\\tools\\pwsh.exe" || view.CommandLine != "pwsh -File run.ps1 --api-key=[redacted]" {
		t.Fatalf("opted-in process details were not included in the view: %+v", view)
	}
}

func TestListForSessionReturnsOnlyProcessesAssociatedWithThatSession(t *testing.T) {
	pid := uint32(44)
	repository := &fakeProcessRepository{
		config: process.DefaultConfig(),
		records: []process.Record{
			{ProcessID: "process-44@100", PID: pid, ImageName: "pwsh.exe", StartedAtNS: 100, OrderAtNS: 300, EndedAtNS: int64Pointer(300)},
			{ProcessID: "process-45@100", PID: 45, ImageName: "pwsh.exe", StartedAtNS: 100, OrderAtNS: 300, EndedAtNS: int64Pointer(300)},
		},
		spans: []correlation.ToolSpan{{
			SessionID: "codex:session-1", SpanID: "span", ToolCallID: "call", ToolName: "shell",
			StartTimeNS: 120, EndTimeNS: 280, ProcessPID: &pid,
		}},
	}

	views, err := NewProcessService(repository, nil).ListForSession(context.Background(), "codex:session-1")
	if err != nil || len(views) != 2 {
		t.Fatalf("session process views = %+v, error = %v", views, err)
	}
	if views[0].ProcessID != "process-44@100" || views[0].AssociationMethod != "explicit_process_pid" ||
		views[0].AssociatedToolCallID == nil || *views[0].AssociatedToolCallID != "call" {
		t.Fatalf("explicit session process association = %+v", views[0])
	}
	if views[1].ProcessID != "process-45@100" || views[1].AssociationMethod != "time_and_category_unique_candidate" ||
		views[1].AssociationConfidence == nil || *views[1].AssociationConfidence != 0.55 {
		t.Fatalf("confidence-labelled session fallback association = %+v", views[1])
	}
}

type fakeProcessRepository struct {
	config  process.Config
	records []process.Record
	spans   []correlation.ToolSpan
	saved   bool
}

func (r *fakeProcessRepository) LoadProcessMonitorConfig(context.Context) (process.Config, error) {
	return r.config, nil
}

func (r *fakeProcessRepository) SaveProcessMonitorConfig(_ context.Context, config process.Config) error {
	r.config = config
	r.saved = true
	return nil
}

func (r *fakeProcessRepository) ListProcesses(context.Context, int, *int64, string) ([]process.Record, error) {
	return r.records, nil
}

func (r *fakeProcessRepository) ListProcessesForSession(context.Context, string) ([]process.Record, error) {
	return r.records, nil
}

func (r *fakeProcessRepository) ListProcessToolSpans(context.Context, int64, int64) ([]correlation.ToolSpan, error) {
	return r.spans, nil
}

func (r *fakeProcessRepository) ListProcessToolSpansForSession(context.Context, string) ([]correlation.ToolSpan, error) {
	return r.spans, nil
}

type fakeProcessMonitor struct {
	config     process.Config
	configured bool
}

func (m *fakeProcessMonitor) Configure(config process.Config) error {
	m.config = config
	m.configured = true
	return nil
}

func (*fakeProcessMonitor) Status() process.MonitorStatus {
	return process.MonitorStatus{State: "sampling"}
}

func int64Pointer(value int64) *int64 { return &value }
