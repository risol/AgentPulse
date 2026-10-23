package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

func TestListSessionsBuildsApplicationViewAndKeepsCursorMetadataPrivate(t *testing.T) {
	startedAt := int64(1_000_000_000)
	lastActivity := int64(2_000_000_000)
	repository := &fakeSessionQueryRepository{sessions: []telemetry.SessionSummary{{
		ID: "codex:session-1", SourceID: telemetry.SourceCodex, ExternalID: "session-1",
		StartedAtNS: &startedAt, LastActivityNS: &lastActivity, Model: "gpt-test",
		EventCount: 3, ToolCallCount: 2, CommandCount: 1, FailedCallCount: 1,
		InputTokens: int64Pointer(12), DataQuality: "observed",
	}}}

	items, err := NewSessionQueryService(repository).ListSessions(context.Background(), SessionListQuery{Range: "all", Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("session list = %+v, error = %v", items, err)
	}
	item := items[0]
	if item.ID != "codex:session-1" || item.SourceID != telemetry.SourceCodex || item.EventCount != 3 ||
		item.ActivityDurationMS == nil || *item.ActivityDurationMS != 1000 || item.ToolCallCount != 2 || item.CommandCount != 1 || item.FailedCallCount != 1 ||
		item.InputTokens == nil || *item.InputTokens != 12 || item.OutputTokens != nil ||
		item.Association != "observed_identifier" || item.DataQuality != "observed" ||
		item.LastActivityNS == nil || *item.LastActivityNS != lastActivity || item.StartedAt == nil || item.LastActivityAt == nil {
		t.Fatalf("session view was not mapped correctly: %+v", item)
	}

	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("last_activity_ns")) {
		t.Fatalf("transport cursor metadata leaked into serialized response: %s", encoded)
	}
}

func TestListSessionsAppliesSupportedFilters(t *testing.T) {
	repository := &fakeSessionQueryRepository{}
	service := NewSessionQueryService(repository)
	now := time.Unix(100_000, 0)
	service.now = func() time.Time { return now }

	_, err := service.ListSessions(context.Background(), SessionListQuery{
		Range: "24h", Model: "  gpt-test ", Limit: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.model != "gpt-test" || repository.sinceNS == nil || *repository.sinceNS != now.Add(-24*time.Hour).UnixNano() || repository.limit != 8 {
		t.Fatalf("session filters were not passed to the repository: %+v", repository)
	}
}

func TestListSessionsRejectsInvalidFilters(t *testing.T) {
	service := NewSessionQueryService(&fakeSessionQueryRepository{})
	if _, err := service.ListSessions(context.Background(), SessionListQuery{Range: "today", Limit: 1}); !errors.Is(err, ErrInvalidSessionQueryRange) {
		t.Fatalf("invalid range error = %v", err)
	}
	if _, err := service.ListSessions(context.Background(), SessionListQuery{Range: "all", Model: string(make([]byte, 129)), Limit: 1}); !errors.Is(err, ErrInvalidSessionModel) {
		t.Fatalf("invalid model error = %v", err)
	}
}

type fakeSessionQueryRepository struct {
	sessions   []telemetry.SessionSummary
	sinceNS    *int64
	model      string
	limit      int
	listCalled bool
}

func (r *fakeSessionQueryRepository) ListSessions(_ context.Context, limit int, _ *int64, _ string, sinceNS *int64, model string) ([]telemetry.SessionSummary, error) {
	r.limit = limit
	r.sinceNS = sinceNS
	r.model = model
	r.listCalled = true
	return r.sessions, nil
}

func (*fakeSessionQueryRepository) SessionTimeline(context.Context, string, int) ([]telemetry.TimelineRecord, error) {
	return nil, nil
}

func (*fakeSessionQueryRepository) ListUnassignedEvents(context.Context, int, int64) ([]telemetry.TimelineRecord, error) {
	return nil, nil
}

func (*fakeSessionQueryRepository) ListToolSummary(context.Context, int64, int64, string) ([]telemetry.ToolSummary, error) {
	return nil, nil
}

func (*fakeSessionQueryRepository) ListToolCalls(context.Context, int64, int64, string, string, string, int, *int64, string) ([]telemetry.ToolCallRecord, error) {
	return nil, nil
}
