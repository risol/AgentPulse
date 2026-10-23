package appapi

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/application"
	"github.com/agentpulse/agentpulse/backend/internal/codexadapter"
	"github.com/agentpulse/agentpulse/backend/internal/process"
	"github.com/agentpulse/agentpulse/backend/internal/storage"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	_ "modernc.org/sqlite"
)

func TestSummaryEndpointRequiresTokenAndAppliesRange(t *testing.T) {
	api, db := startTestAPI(t)
	receivedAt := time.Now().UnixNano()
	if _, err := db.Exec(`INSERT INTO telemetry_events(signal_type,received_at_ns,event_name,attributes_json) VALUES('logs',?,'log_record','{}')`, receivedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO metric_points(metric_name,metric_type,numeric_value,received_at_ns) VALUES('codex.tool.call','sum',1,?)`, receivedAt); err != nil {
		t.Fatal(err)
	}

	unauthorized, err := http.Get("http://" + api.Address() + "/api/v1/dashboard/summary")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized request returned HTTP %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}

	for _, test := range []struct {
		query string
		want  string
	}{
		{query: "", want: "1h"},
		{query: "?range=24h", want: "24h"},
	} {
		request, err := http.NewRequest(http.MethodGet, "http://"+api.Address()+"/api/v1/dashboard/summary"+test.query, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+api.Token())
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Range  string           `json:"range"`
			Events map[string]int64 `json:"events"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || body.Range != test.want || body.Events["logs"] != 1 || body.Events["metrics"] != 1 {
			t.Fatalf("range query %q returned HTTP %d and range %q, want HTTP %d and range %q", test.query, response.StatusCode, body.Range, http.StatusOK, test.want)
		}
	}

	invalid, err := http.NewRequest(http.MethodGet, "http://"+api.Address()+"/api/v1/dashboard/summary?range=all", nil)
	if err != nil {
		t.Fatal(err)
	}
	invalid.Header.Set("Authorization", "Bearer "+api.Token())
	response, err := http.DefaultClient.Do(invalid)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid range returned HTTP %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func TestApplicationAPIRateLimitReturnsRetryAfter(t *testing.T) {
	api, _ := startTestAPI(t)
	api.requestLimiter = newRequestRateLimiter(0, 1, time.Now)

	first := getAuthorized(t, api, "/api/v1/status")
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first request returned HTTP %d, want %d", first.StatusCode, http.StatusOK)
	}

	second := getAuthorized(t, api, "/api/v1/status")
	second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") != "1" {
		t.Fatalf("rate-limited request returned HTTP %d with Retry-After %q", second.StatusCode, second.Header.Get("Retry-After"))
	}
}

func TestStartRequiresComposedApplicationDependencies(t *testing.T) {
	if _, err := Start(Dependencies{}); err == nil || !strings.Contains(err.Error(), "missing application dependency") {
		t.Fatalf("Start with missing dependencies returned error %v", err)
	}
}

func TestCodexConfigurationEndpointsUseInjectedApplicationPort(t *testing.T) {
	api, _ := startTestAPI(t)
	configResponse := getAuthorized(t, api, "/api/v1/config/codex")
	var info application.CodexConfigurationInfo
	if err := json.NewDecoder(configResponse.Body).Decode(&info); err != nil {
		configResponse.Body.Close()
		t.Fatal(err)
	}
	configResponse.Body.Close()
	if configResponse.StatusCode != http.StatusOK || info.Path != "C:\\test\\config.toml" || !info.Configured {
		t.Fatalf("Codex config response HTTP %d: %+v", configResponse.StatusCode, info)
	}

	previewResponse := getAuthorized(t, api, "/api/v1/config/codex/preview")
	var preview application.CodexConfigurationPreview
	if err := json.NewDecoder(previewResponse.Body).Decode(&preview); err != nil {
		previewResponse.Body.Close()
		t.Fatal(err)
	}
	previewResponse.Body.Close()
	if previewResponse.StatusCode != http.StatusOK || !preview.Restart || preview.Changes["otel.log_user_prompt"] != "false" {
		t.Fatalf("Codex preview response HTTP %d: %+v", previewResponse.StatusCode, preview)
	}

	applyResponse := authorizedRequest(t, api, http.MethodPost, "/api/v1/config/codex/apply", "")
	defer applyResponse.Body.Close()
	if applyResponse.StatusCode != http.StatusOK {
		t.Fatalf("Codex apply response HTTP %d", applyResponse.StatusCode)
	}
	restoreResponse := authorizedRequest(t, api, http.MethodPost, "/api/v1/config/codex/restore", `{"backup":"config.toml.backup.test"}`)
	defer restoreResponse.Body.Close()
	if restoreResponse.StatusCode != http.StatusOK {
		t.Fatalf("Codex restore response HTTP %d", restoreResponse.StatusCode)
	}
}

func TestReceiverPortSettingsAreValidatedAndRequireRestartOnlyOnChange(t *testing.T) {
	api, db := startTestAPI(t)
	response := getAuthorized(t, api, "/api/v1/settings/receiver")
	var settings application.ReceiverSettings
	if err := json.NewDecoder(response.Body).Decode(&settings); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || settings.Port != application.DefaultOTLPPort {
		t.Fatalf("receiver settings response HTTP %d: %+v", response.StatusCode, settings)
	}

	update := authorizedRequest(t, api, http.MethodPut, "/api/v1/settings/receiver", `{"port":4545}`)
	var result struct {
		Settings        application.ReceiverSettings `json:"settings"`
		RestartRequired bool                         `json:"restart_required"`
	}
	if err := json.NewDecoder(update.Body).Decode(&result); err != nil {
		update.Body.Close()
		t.Fatal(err)
	}
	update.Body.Close()
	if update.StatusCode != http.StatusOK || result.Settings.Port != 4545 || !result.RestartRequired {
		t.Fatalf("receiver settings update HTTP %d: %+v", update.StatusCode, result)
	}

	samePort := authorizedRequest(t, api, http.MethodPut, "/api/v1/settings/receiver", `{"port":4545}`)
	var unchanged struct {
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.NewDecoder(samePort.Body).Decode(&unchanged); err != nil {
		samePort.Body.Close()
		t.Fatal(err)
	}
	samePort.Body.Close()
	if samePort.StatusCode != http.StatusOK || unchanged.RestartRequired {
		t.Fatalf("unchanged receiver port update HTTP %d: %+v", samePort.StatusCode, unchanged)
	}

	invalid := authorizedRequest(t, api, http.MethodPut, "/api/v1/settings/receiver", `{"port":80}`)
	invalid.Body.Close()
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("privileged port update returned HTTP %d, want %d", invalid.StatusCode, http.StatusBadRequest)
	}
	stored, err := application.NewReceiverSettingsService(storage.NewSQLite(db)).Load(context.Background())
	if err != nil || stored.Port != 4545 {
		t.Fatalf("invalid update changed receiver port: settings=%+v error=%v", stored, err)
	}
}

func TestReceiverPortConflictDoesNotPersistTheUnavailablePort(t *testing.T) {
	api, db := startTestAPIWithPortCheck(t, func(port int) error {
		if port == 4545 {
			return errors.New("address already in use")
		}
		return nil
	})
	response := authorizedRequest(t, api, http.MethodPut, "/api/v1/settings/receiver", `{"port":4545}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("unavailable port update returned HTTP %d, want %d", response.StatusCode, http.StatusConflict)
	}
	stored, err := application.NewReceiverSettingsService(storage.NewSQLite(db)).Load(context.Background())
	if err != nil || stored.Port != application.DefaultOTLPPort {
		t.Fatalf("port conflict changed stored receiver port: settings=%+v error=%v", stored, err)
	}
}

func TestAnalyticsTrendsReturnNormalizedTokenAndDurationBuckets(t *testing.T) {
	api, db := startTestAPI(t)
	now := time.Now().UnixNano()
	observedAt := now - int64(20*time.Second)
	withoutTokensAt := now - int64(10*time.Second)
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: now,
		Events: []telemetry.Event{
			{EventTimeNS: &observedAt, TraceID: "trend-trace", SpanID: "model-span", Name: "usage", Attributes: map[string]any{
				"conversation.id": "trend-session", "input_token_count": int64(12), "output_token_count": int64(7), "ttft_ms": float64(25),
			}},
			{EventTimeNS: &withoutTokensAt, Name: "heartbeat", Attributes: map[string]any{"conversation.id": "trend-session"}},
			{EventTimeNS: &observedAt, Name: "tool_call", Attributes: map[string]any{
				"tool_name": "shell", "tool.call_id": "trend-call", "success": true, "duration_ms": float64(125),
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	modelSpanEnd := observedAt + int64(time.Second)
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalTraces, ReceivedAtNS: now,
		Spans: []telemetry.Span{{
			TraceID: "trend-trace", SpanID: "model-span", StartTimeNS: observedAt, EndTimeNS: modelSpanEnd, StatusCode: 1,
			Attributes: map[string]any{"model": "gpt-test", "conversation.id": "trend-session"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	response := getAuthorized(t, api, "/api/v1/analytics/trends?range=1h")
	var view struct {
		Range                 string `json:"range"`
		BucketIntervalSeconds int64  `json:"bucket_interval_seconds"`
		Points                []struct {
			HasData               bool     `json:"has_data"`
			InputTokens           *int64   `json:"input_tokens"`
			InputTokenSamples     int64    `json:"input_token_samples"`
			OutputTokens          *int64   `json:"output_tokens"`
			TTFTSamples           int64    `json:"ttft_samples"`
			AverageTTFTMS         *float64 `json:"average_ttft_ms"`
			RequestRateSamples    int64    `json:"request_rate_samples"`
			AverageRequestRateTPS *float64 `json:"average_request_rate_tokens_per_second"`
			ToolCallCount         int64    `json:"tool_call_count"`
			DurationSamples       int64    `json:"duration_samples"`
			AverageDurationMS     *float64 `json:"average_duration_ms"`
		} `json:"points"`
	}
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || view.Range != "1h" || view.BucketIntervalSeconds != 60 || len(view.Points) < 50 {
		t.Fatalf("unexpected trends response: HTTP %d, range=%q interval=%d points=%d", response.StatusCode, view.Range, view.BucketIntervalSeconds, len(view.Points))
	}
	found := false
	foundGap := false
	for _, point := range view.Points {
		if point.HasData && point.InputTokens != nil && *point.InputTokens == 12 {
			found = point.InputTokenSamples == 1 && point.OutputTokens != nil && *point.OutputTokens == 7 &&
				point.TTFTSamples == 1 && point.AverageTTFTMS != nil && *point.AverageTTFTMS == 25 &&
				point.RequestRateSamples == 1 && point.AverageRequestRateTPS != nil && *point.AverageRequestRateTPS == 7 &&
				point.ToolCallCount == 1 && point.DurationSamples == 1 && point.AverageDurationMS != nil && *point.AverageDurationMS == 125
		}
		if !point.HasData && point.InputTokens == nil && point.OutputTokens == nil && point.AverageDurationMS == nil {
			foundGap = true
		}
	}
	if !found || !foundGap {
		t.Fatalf("trends did not preserve observed values and empty buckets: found=%t gap=%t", found, foundGap)
	}
	response = getAuthorized(t, api, "/api/v1/analytics/trends?range=unknown")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid trend range returned HTTP %d", response.StatusCode)
	}
}

func TestSessionAndTimelineQueriesUseObservedData(t *testing.T) {
	api, db := startTestAPI(t)
	now := time.Now().UnixNano()
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: now,
		Events: []telemetry.Event{{EventTimeNS: &now, Name: "log_record", Attributes: map[string]any{
			"conversation.id": "session-1", "model": "gpt-test", "input_token_count": int64(32),
			"output_token_count": int64(12),
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	modelStart := now - int64(time.Second)
	toolStart := now + int64(time.Second)
	toolEnd := toolStart + int64(500*time.Millisecond)
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalTraces, ReceivedAtNS: now,
		Spans: []telemetry.Span{
			{TraceID: "timeline-trace", SpanID: "model-span", StartTimeNS: modelStart, EndTimeNS: now + int64(500*time.Millisecond), Attributes: map[string]any{
				"conversation.id": "session-1", "model": "gpt-test",
			}},
			{TraceID: "timeline-trace", SpanID: "tool-span", StartTimeNS: toolStart, EndTimeNS: toolEnd, StatusCode: 1, Attributes: map[string]any{
				"conversation.id": "session-1", "tool.name": "shell", "tool.call_id": "call-1", "process.pid": int64(123),
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSQLite(db).SessionTimeline(context.Background(), "codex:session-1", 10); err != nil {
		t.Fatalf("query session timeline repository: %v", err)
	}

	response := getAuthorized(t, api, "/api/v1/sessions?limit=10")
	var sessions struct {
		Items []struct {
			ID               string   `json:"id"`
			EventCount       int64    `json:"event_count"`
			ActivityDuration *float64 `json:"activity_duration_ms"`
			ToolCallCount    int64    `json:"tool_call_count"`
			CommandCount     int64    `json:"command_count"`
			FailedCallCount  int64    `json:"failed_call_count"`
			InputTokens      *int64   `json:"input_tokens"`
			Association      string   `json:"association"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sessions); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(sessions.Items) != 1 {
		t.Fatalf("sessions returned HTTP %d and %d items, want HTTP %d and 1 item", response.StatusCode, len(sessions.Items), http.StatusOK)
	}
	session := sessions.Items[0]
	if session.ID != "codex:session-1" || session.EventCount != 1 || session.InputTokens == nil || *session.InputTokens != 32 ||
		session.ActivityDuration == nil || *session.ActivityDuration <= 0 || session.ToolCallCount != 1 || session.CommandCount != 1 || session.FailedCallCount != 0 ||
		session.Association != "observed_identifier" {
		t.Fatalf("unexpected session result: %+v", session)
	}

	response = getAuthorized(t, api, "/api/v1/sessions/codex:session-1/timeline?limit=10")
	var timeline struct {
		SessionID string           `json:"session_id"`
		Items     []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&timeline); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || timeline.SessionID != "codex:session-1" || len(timeline.Items) != 3 {
		t.Fatalf("unexpected timeline response: HTTP %d, %+v", response.StatusCode, timeline)
	}
	if timeline.Items[0]["record_type"] != "span" || timeline.Items[0]["kind"] != "model_request" ||
		timeline.Items[1]["record_type"] != "event" || timeline.Items[1]["signal"] != "logs" ||
		timeline.Items[2]["kind"] != "tool_call" || timeline.Items[2]["process_pid"] != float64(123) ||
		timeline.Items[2]["duration_ms"] != 500.0 {
		t.Fatalf("timeline did not merge and map event/span records: %+v", timeline.Items)
	}
}

func TestSessionListCursorReturnsNonOverlappingPages(t *testing.T) {
	api, db := startTestAPI(t)
	now := time.Now().UnixNano()
	events := make([]telemetry.Event, 0, 3)
	for index, sessionID := range []string{"session-a", "session-b", "session-c"} {
		timestamp := now + int64(index)
		events = append(events, telemetry.Event{EventTimeNS: &timestamp, Name: "log_record", Attributes: map[string]any{
			"conversation.id": sessionID, "model": "gpt-test",
		}})
	}
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: now, Events: events,
	}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/sessions?limit=2")
	var firstPage struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&firstPage); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(firstPage.Items) != 2 || firstPage.Items[0].ID != "codex:session-c" || firstPage.Items[1].ID != "codex:session-b" || firstPage.NextCursor == "" {
		t.Fatalf("unexpected first session page: HTTP %d, %+v", response.StatusCode, firstPage)
	}

	response = getAuthorized(t, api, "/api/v1/sessions?limit=2&cursor="+firstPage.NextCursor)
	var secondPage struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&secondPage); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(secondPage.Items) != 1 || secondPage.Items[0].ID != "codex:session-a" {
		t.Fatalf("unexpected second session page: HTTP %d, %+v", response.StatusCode, secondPage)
	}
}

func TestSessionListFiltersByActivityRangeAndModel(t *testing.T) {
	api, db := startTestAPI(t)
	now := time.Now().UnixNano()
	recentAt := now - int64(time.Hour)
	oldAt := now - int64(48*time.Hour)
	if err := codexadapter.NewWriter(storage.NewSQLite(db)).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: now,
		Events: []telemetry.Event{
			{Name: "log_record", EventTimeNS: &recentAt, Attributes: map[string]any{"conversation.id": "recent-session", "model": "gpt-5-codex"}},
			{Name: "log_record", EventTimeNS: &oldAt, Attributes: map[string]any{"conversation.id": "old-session", "model": "gpt-4.1"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/sessions?range=24h&model=gpt-5&limit=10")
	var result struct {
		Range string `json:"range"`
		Model string `json:"model"`
		Items []struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || result.Range != "24h" || result.Model != "gpt-5" || len(result.Items) != 1 ||
		result.Items[0].ID != "codex:recent-session" || result.Items[0].Model != "gpt-5-codex" {
		t.Fatalf("session filters returned HTTP %d: %+v", response.StatusCode, result)
	}

	response = getAuthorized(t, api, "/api/v1/sessions?range=today")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid session range returned HTTP %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func TestSessionProcessEndpointReturnsAssociatedProcessRecords(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	pid := int64(44)
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalTraces, ReceivedAtNS: 400,
		Spans: []telemetry.Span{{
			TraceID: "session-process-trace", SpanID: "session-process-span",
			StartTimeNS: 100, EndTimeNS: 300,
			Attributes: map[string]any{
				"conversation.id": "session-process", "tool.name": "shell",
				"tool.call_id": "session-process-call", "process.pid": pid,
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	sample := process.Sample{
		ProcessID: process.InstanceID(uint32(pid), 90), PID: uint32(pid),
		ParentPID: 10, ImageName: "pwsh.exe", StartedAtNS: 90, SampledAtNS: 310,
	}
	if err := store.ApplyProcessSnapshot(context.Background(), process.Snapshot{
		SampledAtNS: 310, Processes: []process.Sample{sample},
	}, process.ChangeSet{}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/sessions/codex:session-process/processes")
	defer response.Body.Close()
	var result struct {
		SessionID string `json:"session_id"`
		Items     []struct {
			PID                   uint32   `json:"pid"`
			AssociationMethod     string   `json:"association_method"`
			AssociationConfidence *float64 `json:"association_confidence"`
			ToolCallID            *string  `json:"associated_tool_call_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.SessionID != "codex:session-process" || len(result.Items) != 1 {
		t.Fatalf("session process response returned HTTP %d: %+v", response.StatusCode, result)
	}
	item := result.Items[0]
	confidence := -1.0
	if item.AssociationConfidence != nil {
		confidence = *item.AssociationConfidence
	}
	toolCallID := ""
	if item.ToolCallID != nil {
		toolCallID = *item.ToolCallID
	}
	if item.PID != uint32(pid) || item.AssociationMethod != "explicit_process_pid" ||
		confidence != 1 || toolCallID != "session-process-call" {
		t.Fatalf("session process association was not exposed: pid=%d method=%s confidence=%g call=%q", item.PID, item.AssociationMethod, confidence, toolCallID)
	}
}

func TestUnassignedEventsAndToolSummaryUsePersistedRecords(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	now := time.Now().UnixNano()
	firstAt := now - int64(2*time.Minute)
	secondAt := now - int64(time.Minute)
	unassignedAt := now - int64(30*time.Second)
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs, ReceivedAtNS: now,
		Events: []telemetry.Event{
			{EventTimeNS: &firstAt, Name: "tool_call", Attributes: map[string]any{
				"conversation.id": "session-tools", "turn.id": "turn-1", "tool_name": "shell",
				"tool.call_id": "call-1", "input_token_count": int64(20), "output_token_count": int64(10),
			}},
			{EventTimeNS: &secondAt, Name: "tool_call", Attributes: map[string]any{
				"conversation.id": "session-tools", "turn.id": "turn-1", "tool_name": "shell",
				"tool.call_id": "call-2", "success": false, "duration_ms": float64(50), "retry_index": int64(1),
			}},
			{EventTimeNS: &unassignedAt, Name: "orphan_turn", Attributes: map[string]any{"turn.id": "unlinked-turn"}},
			{Name: "orphan_event", Attributes: map[string]any{"model": "gpt-test"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	span := telemetry.Span{
		TraceID: "trace-1", SpanID: "span-1", StartTimeNS: firstAt,
		EndTimeNS: firstAt + int64(100*time.Millisecond), StatusCode: 1,
		Attributes: map[string]any{
			"conversation.id": "session-tools", "turn.id": "turn-1", "tool.name": "shell",
			"tool.call_id": "call-1",
		},
	}
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		SourceID: telemetry.SourceCodex, Signal: telemetry.SignalTraces, ReceivedAtNS: now, Spans: []telemetry.Span{span},
	}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/events/unassigned?limit=1")
	var unassigned struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&unassigned); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(unassigned.Items) != 1 || unassigned.NextCursor == "" || unassigned.Items[0]["data_quality"] != "partial" {
		t.Fatalf("unexpected unassigned event page: HTTP %d, %+v", response.StatusCode, unassigned)
	}
	response = getAuthorized(t, api, "/api/v1/events/unassigned?limit=10&cursor="+unassigned.NextCursor)
	var remaining struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&remaining); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(remaining.Items) != 1 || remaining.Items[0]["turn_id"] != "codex:unlinked-turn" {
		t.Fatalf("unexpected remaining unassigned events: HTTP %d, %+v", response.StatusCode, remaining)
	}

	response = getAuthorized(t, api, "/api/v1/tools/summary?range=1h&session_id=codex:session-tools")
	var summary struct {
		Items []telemetry.ToolSummary `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(summary.Items) != 1 {
		t.Fatalf("unexpected tool summary response: HTTP %d, %+v", response.StatusCode, summary)
	}
	tool := summary.Items[0]
	if tool.ToolName != "shell" || tool.TotalCalls != 2 || tool.SuccessCalls != 1 || tool.FailureCalls != 1 ||
		tool.FailureRate == nil || *tool.FailureRate != 0.5 || tool.DurationSamples != 2 ||
		tool.P50DurationMS == nil || *tool.P50DurationMS != 50 || tool.P95DurationMS == nil || *tool.P95DurationMS != 100 {
		t.Fatalf("unexpected tool summary: %+v", tool)
	}

	response = getAuthorized(t, api, "/api/v1/tools/calls?range=1h&session_id=codex%3Asession-tools&limit=1")
	var firstCalls struct {
		Items      []telemetry.ToolCallRecord `json:"items"`
		NextCursor string                     `json:"next_cursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&firstCalls); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(firstCalls.Items) != 1 || firstCalls.Items[0].ExternalID == nil ||
		*firstCalls.Items[0].ExternalID != "call-2" || firstCalls.Items[0].RetryIndex == nil || *firstCalls.Items[0].RetryIndex != 1 ||
		firstCalls.NextCursor == "" {
		t.Fatalf("unexpected first tool call page: HTTP %d, %+v", response.StatusCode, firstCalls)
	}
	response = getAuthorized(t, api, "/api/v1/tools/calls?range=1h&session_id=codex%3Asession-tools&limit=1&cursor="+firstCalls.NextCursor)
	var secondCalls struct {
		Items []telemetry.ToolCallRecord `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&secondCalls); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(secondCalls.Items) != 1 || secondCalls.Items[0].ExternalID == nil || *secondCalls.Items[0].ExternalID != "call-1" {
		t.Fatalf("unexpected second tool call page: HTTP %d, %+v", response.StatusCode, secondCalls)
	}
	response = getAuthorized(t, api, "/api/v1/tools/calls?range=1h&outcome=failed")
	var failedCalls struct {
		Items []telemetry.ToolCallRecord `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&failedCalls); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(failedCalls.Items) != 1 || failedCalls.Items[0].Status == nil || *failedCalls.Items[0].Status != "error" {
		t.Fatalf("unexpected failed call filter: HTTP %d, %+v", response.StatusCode, failedCalls)
	}

	response = getAuthorized(t, api, "/api/v1/sessions/codex:session-tools/analysis")
	var analysis struct {
		ActivityWindowMS *float64                `json:"activity_window_ms"`
		DataQuality      string                  `json:"data_quality"`
		InputTokens      *int64                  `json:"input_tokens"`
		Tools            []telemetry.ToolSummary `json:"tools"`
		SourceEventIDs   []int64                 `json:"source_event_ids"`
		Durations        map[string]any          `json:"durations"`
		Tokens           map[string]any          `json:"tokens"`
	}
	if err := json.NewDecoder(response.Body).Decode(&analysis); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || analysis.ActivityWindowMS == nil || *analysis.ActivityWindowMS != 60_000 ||
		analysis.DataQuality != "partial" || len(analysis.Tools) != 1 || analysis.Tools[0].TotalCalls != 2 ||
		len(analysis.SourceEventIDs) != 2 || analysis.InputTokens == nil {
		t.Fatalf("unexpected session analysis response: HTTP %d, %+v", response.StatusCode, analysis)
	}
}

func TestProcessMonitorSettingsAreValidatedAndStored(t *testing.T) {
	api, _ := startTestAPI(t)
	request, err := http.NewRequest(http.MethodPut, "http://"+api.Address()+"/api/v1/settings/process-monitor", strings.NewReader(`{"enabled":true,"sample_interval_ms":500,"root_process_names":["Codex"],"capture_executable_paths":true,"capture_command_lines":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Config process.Config `json:"config"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !body.Config.Enabled || body.Config.RootProcessNames[0] != "codex.exe" || body.Config.RetentionDays != process.DefaultRetentionDays || !body.Config.CaptureExecutablePaths || !body.Config.CaptureCommandLines {
		t.Fatalf("process settings response: HTTP %d, %+v", response.StatusCode, body.Config)
	}

	request, err = http.NewRequest(http.MethodPut, "http://"+api.Address()+"/api/v1/settings/process-monitor", strings.NewReader(`{"enabled":true,"sample_interval_ms":100,"root_process_names":["Codex"]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid process sampling interval returned HTTP %d, want %d", response.StatusCode, http.StatusBadRequest)
	}

	response = getAuthorized(t, api, "/api/v1/processes?limit=20")
	var processes struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&processes); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(processes.Items) != 0 {
		t.Fatalf("empty process list returned HTTP %d and %d rows", response.StatusCode, len(processes.Items))
	}
}

func TestDataRetentionSettingsAreAuthenticatedAndValidated(t *testing.T) {
	api, db := startTestAPI(t)
	response := getAuthorized(t, api, "/api/v1/settings/data-retention")
	var initial struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(response.Body).Decode(&initial); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || initial.RetentionDays != 30 {
		t.Fatalf("default retention response = HTTP %d, %d days; want HTTP %d, 30 days", response.StatusCode, initial.RetentionDays, http.StatusOK)
	}

	request, err := http.NewRequest(http.MethodPut, "http://"+api.Address()+"/api/v1/settings/data-retention", strings.NewReader(`{"retention_days":14}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var updated struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || updated.RetentionDays != 14 {
		t.Fatalf("updated retention response = HTTP %d, %d days; want HTTP %d, 14 days", response.StatusCode, updated.RetentionDays, http.StatusOK)
	}
	var stored string
	if err := db.QueryRow(`SELECT value FROM app_state WHERE key='telemetry_retention_days'`).Scan(&stored); err != nil || stored != "14" {
		t.Fatalf("stored retention = %q, error = %v; want 14", stored, err)
	}

	request, err = http.NewRequest(http.MethodPut, "http://"+api.Address()+"/api/v1/settings/data-retention", strings.NewReader(`{"retention_days":366}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("out-of-range retention returned HTTP %d, want %d", response.StatusCode, http.StatusBadRequest)
	}

	request, err = http.NewRequest(http.MethodPost, "http://"+api.Address()+"/api/v1/settings/data-retention/cleanup", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var cleanup struct {
		Scheduled bool `json:"cleanup_scheduled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&cleanup); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || !cleanup.Scheduled {
		t.Fatalf("cleanup response = HTTP %d, scheduled %t; want HTTP %d and scheduled", response.StatusCode, cleanup.Scheduled, http.StatusAccepted)
	}
}

func TestDatabaseExportContainsConsistentSQLiteSnapshot(t *testing.T) {
	api, db := startTestAPI(t)
	if _, err := db.Exec(`INSERT INTO telemetry_events(signal_type,received_at_ns,event_name,attributes_json) VALUES('logs',?,'redacted_log','{}')`, time.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}
	response := getAuthorized(t, api, "/api/v1/data/export")
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/vnd.sqlite3" {
		response.Body.Close()
		t.Fatalf("database export returned HTTP %d with content type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	contents, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "agentpulse-export-*.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	exported, err := sql.Open("sqlite", file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer exported.Close()
	var version, count int
	if err := exported.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := exported.QueryRow(`SELECT COUNT(*) FROM telemetry_events WHERE event_name='redacted_log'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if version != storage.NewSQLite(db).SchemaVersion() || count != 1 {
		t.Fatalf("exported snapshot schema/data = %d/%d, want %d/1", version, count, storage.NewSQLite(db).SchemaVersion())
	}
}

func TestClearHistoryEndpointDeletesDataButKeepsSettings(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		Signal: telemetry.SignalLogs, ReceivedAtNS: time.Now().UnixNano(),
		Events: []telemetry.Event{{Name: "log_record", Attributes: map[string]any{"conversation.id": "clear-api-session"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTelemetryRetentionDays(context.Background(), 21); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodDelete, "http://"+api.Address()+"/api/v1/data/history", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		DeletedRows int64 `json:"deleted_rows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM telemetry_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	days, err := store.LoadTelemetryRetentionDays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.DeletedRows == 0 || events != 0 || days != 21 {
		t.Fatalf("clear response HTTP %d deleted=%d events=%d retention=%d; want data cleared and settings retained", response.StatusCode, result.DeletedRows, events, days)
	}
}

func TestEventStreamPublishesAuthenticatedTelemetryUpdates(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := storage.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	bus := telemetry.NewChangeBus()
	dependencies := testDependencies(db)
	dependencies.Changes = bus
	api, err := Start(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+api.Address()+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream returned HTTP %d with content type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	ready, err := reader.ReadString('\n')
	if err != nil || ready != "event: ready\n" {
		t.Fatalf("stream ready line = %q, error = %v", ready, err)
	}
	if _, err := reader.ReadString('\n'); err != nil { // initial empty data field
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil { // frame delimiter
		t.Fatal(err)
	}
	bus.Publish(telemetry.Update{Signal: telemetry.SignalLogs, ReceivedAtNS: time.Now().UnixNano()})
	var eventLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		if strings.HasPrefix(line, "event:") {
			eventLine = strings.TrimSpace(line)
		}
		if strings.HasPrefix(line, "data:") {
			dataLine = strings.TrimSpace(line)
		}
	}
	if eventLine != "event: telemetry" || !strings.Contains(dataLine, `"signal":"logs"`) {
		t.Fatalf("unexpected stream update: %q %q", eventLine, dataLine)
	}
}

func TestIngestDiagnosticsEndpointReturnsOnlySanitizedCodes(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	if err := store.RecordIngestDiagnostic(context.Background(), telemetry.SignalTraces, "invalid_protobuf"); err != nil {
		t.Fatal(err)
	}
	response := getAuthorized(t, api, "/api/v1/diagnostics/ingest")
	var snapshot struct {
		CountLast24Hours int64 `json:"count_last_24_hours"`
		Recent           []struct {
			Signal string `json:"signal"`
			Code   string `json:"code"`
		} `json:"recent"`
	}
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || snapshot.CountLast24Hours != 1 || len(snapshot.Recent) != 1 || snapshot.Recent[0].Signal != "traces" || snapshot.Recent[0].Code != "invalid_protobuf" {
		t.Fatalf("diagnostics response HTTP %d: %+v", response.StatusCode, snapshot)
	}
}

func TestStatusIncludesIngestDiagnosticsAndQueueCounters(t *testing.T) {
	_, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	if err := store.RecordIngestDiagnostic(context.Background(), telemetry.SignalMetrics, "request_too_large"); err != nil {
		t.Fatal(err)
	}
	dependencies := testDependencies(db)
	dependencies.IngestQueue = testQueueStatus{depth: 2, capacity: 4, rejected: 3}
	dependencies.BackendVersion = "test-build"
	withQueue, err := Start(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = withQueue.Close() })
	response := getAuthorized(t, withQueue, "/api/v1/status")
	defer response.Body.Close()
	var body struct {
		BackendVersion string `json:"backend_version"`
		SchemaVersion  int    `json:"schema_version"`
		Diagnostics    struct {
			Count int64 `json:"count_last_24_hours"`
		} `json:"ingest_diagnostics"`
		Queue struct {
			Depth    int    `json:"depth"`
			Capacity int    `json:"capacity"`
			Rejected uint64 `json:"rejected"`
		} `json:"ingest_queue"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || body.BackendVersion != "test-build" || body.SchemaVersion != storage.NewSQLite(db).SchemaVersion() || body.Diagnostics.Count != 1 || body.Queue.Depth != 2 || body.Queue.Capacity != 4 || body.Queue.Rejected != 3 {
		t.Fatalf("status returned HTTP %d with version=%q schema=%d diagnostics=%d and queue=%+v", response.StatusCode, body.BackendVersion, body.SchemaVersion, body.Diagnostics.Count, body.Queue)
	}
}

func TestApplicationLogCleanupEndpointReportsRemovedBytes(t *testing.T) {
	_, db := startTestAPI(t)
	dependencies := testDependencies(db)
	dependencies.BackendVersion = "test-build"
	dependencies.LogCleaner = testLogCleaner{deletedBytes: 4096}
	api, err := Start(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.Close() })
	request, err := http.NewRequest(http.MethodPost, "http://"+api.Address()+"/api/v1/settings/logs/cleanup", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		DeletedBytes  int64 `json:"deleted_bytes"`
		RetentionDays int   `json:"retention_days"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.DeletedBytes != 4096 || result.RetentionDays != 14 {
		t.Fatalf("log cleanup returned HTTP %d with result %+v", response.StatusCode, result)
	}
}

type testQueueStatus struct {
	depth    int
	capacity int
	rejected uint64
}

func (q testQueueStatus) Depth() int       { return q.depth }
func (q testQueueStatus) Capacity() int    { return q.capacity }
func (q testQueueStatus) Rejected() uint64 { return q.rejected }

type testLogCleaner struct{ deletedBytes int64 }

func (c testLogCleaner) CleanupExpired(context.Context) (int64, error) {
	return c.deletedBytes, nil
}

func TestProcessListUsesConfiguredExactCategoryRules(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	config := process.DefaultConfig()
	config.CategoryRules = []process.CategoryRule{{Category: "tooling", ProcessNames: []string{"runner.exe"}}}
	if err := store.SaveProcessMonitorConfig(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyProcessSnapshot(context.Background(), process.Snapshot{
		SampledAtNS: 200,
		Processes:   []process.Sample{{ProcessID: "10:100", PID: 10, ImageName: "Runner.EXE", StartedAtNS: 100, SampledAtNS: 200}},
	}, process.ChangeSet{}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/processes")
	defer response.Body.Close()
	var body struct {
		Items []struct {
			Category         *string  `json:"command_category"`
			CategoryMethod   *string  `json:"category_method"`
			Association      string   `json:"association_method"`
			AssociationScore *float64 `json:"association_confidence"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body.Items) != 1 {
		t.Fatalf("process list returned HTTP %d with %d rows", response.StatusCode, len(body.Items))
	}
	item := body.Items[0]
	if item.Category == nil || *item.Category != "tooling" || item.CategoryMethod == nil || *item.CategoryMethod != "configured_image_name_rule" {
		t.Fatalf("configured category was not exposed: %+v", item)
	}
	if item.Association != "unassigned" || item.AssociationScore != nil {
		t.Fatalf("name category was incorrectly reported as a tool-call association: %+v", item)
	}
}

func TestProcessListExposesExplicitToolAssociationAndConfidence(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	if err := store.ApplyProcessSnapshot(context.Background(), process.Snapshot{
		SampledAtNS: 300,
		Processes:   []process.Sample{{ProcessID: "44:100", PID: 44, ImageName: "pwsh.exe", StartedAtNS: 100, SampledAtNS: 300}},
	}, process.ChangeSet{}); err != nil {
		t.Fatal(err)
	}
	if err := codexadapter.NewWriter(store).SaveTelemetry(context.Background(), telemetry.Batch{
		Signal: telemetry.SignalTraces, ReceivedAtNS: 300,
		Spans: []telemetry.Span{{TraceID: "trace", SpanID: "span", StartTimeNS: 120, EndTimeNS: 280, Attributes: map[string]any{
			"tool.name": "shell", "tool.call_id": "call-1", "process.pid": int64(44),
		}}},
	}); err != nil {
		t.Fatal(err)
	}

	response := getAuthorized(t, api, "/api/v1/processes")
	defer response.Body.Close()
	var body struct {
		Items []struct {
			Category         *string  `json:"command_category"`
			Association      string   `json:"association_method"`
			AssociationScore *float64 `json:"association_confidence"`
			ToolCallID       *string  `json:"associated_tool_call_id"`
			SpanID           *string  `json:"associated_span_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body.Items) != 1 {
		t.Fatalf("process list returned HTTP %d with %d rows", response.StatusCode, len(body.Items))
	}
	item := body.Items[0]
	if item.Category == nil || *item.Category != "shell" || item.Association != "explicit_process_pid" || item.AssociationScore == nil || *item.AssociationScore != 1 || item.ToolCallID == nil || *item.ToolCallID != "call-1" || item.SpanID == nil || *item.SpanID != "span" {
		t.Fatalf("explicit association was not exposed: %+v", item)
	}
}

func TestProcessListCursorReturnsNonOverlappingPages(t *testing.T) {
	api, db := startTestAPI(t)
	store := storage.NewSQLite(db)
	for index, startedAt := range []int64{100, 200, 300} {
		pid := uint32(20 + index)
		sample := process.Sample{ProcessID: process.InstanceID(pid, startedAt), PID: pid, ImageName: "runner.exe", StartedAtNS: startedAt, SampledAtNS: startedAt + 10}
		snapshot := process.Snapshot{SampledAtNS: sample.SampledAtNS, Processes: []process.Sample{sample}}
		if err := store.ApplyProcessSnapshot(context.Background(), snapshot, process.ChangeSet{}); err != nil {
			t.Fatal(err)
		}
	}

	response := getAuthorized(t, api, "/api/v1/processes?limit=2")
	var first struct {
		Items []struct {
			ID string `json:"process_id"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&first); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first process page returned HTTP %d: %+v", response.StatusCode, first)
	}

	response = getAuthorized(t, api, "/api/v1/processes?limit=2&cursor="+*first.NextCursor)
	var second struct {
		Items []struct {
			ID string `json:"process_id"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&second); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID || second.NextCursor != nil {
		t.Fatalf("second process page returned HTTP %d: %+v", response.StatusCode, second)
	}

	response = getAuthorized(t, api, "/api/v1/processes?cursor=invalid")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid process cursor returned HTTP %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func getAuthorized(t *testing.T, api *Server, path string) *http.Response {
	return authorizedRequest(t, api, http.MethodGet, path, "")
}

func authorizedRequest(t *testing.T, api *Server, method, path, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, "http://"+api.Address()+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+api.Token())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func startTestAPI(t *testing.T) (*Server, *sql.DB) {
	return startTestAPIWithPortCheck(t, func(int) error { return nil })
}

func startTestAPIWithPortCheck(t *testing.T, checkReceiverPort func(int) error) (*Server, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := storage.Initialize(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	dependencies := testDependencies(db)
	dependencies.CheckReceiverPort = checkReceiverPort
	api, err := Start(dependencies)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = api.Close()
		_ = db.Close()
	})
	return api, db
}

func testDependencies(db *sql.DB) Dependencies {
	store := storage.NewSQLite(db)
	return Dependencies{
		Process:            application.NewProcessService(store, nil),
		SessionQueries:     application.NewSessionQueryService(store),
		SessionAnalytics:   application.NewSessionAnalyticsService(store),
		Trends:             application.NewAnalyticsTrendService(store),
		Retention:          application.NewTelemetryRetentionService(store),
		IngestDiagnostics:  application.NewIngestDiagnosticService(store),
		TelemetryOverview:  application.NewTelemetryOverviewService(store),
		TelemetryData:      application.NewTelemetryDataService(store),
		ReceiverSettings:   application.NewReceiverSettingsService(store),
		CheckReceiverPort:  func(int) error { return nil },
		CodexConfiguration: testCodexConfiguration{},
		ReceiverAddress:    "127.0.0.1:4318",
	}
}

type testCodexConfiguration struct{}

func (testCodexConfiguration) Detect() (application.CodexConfigurationInfo, error) {
	return application.CodexConfigurationInfo{Path: "C:\\test\\config.toml", Exists: true, Configured: true}, nil
}

func (testCodexConfiguration) Preview() (application.CodexConfigurationPreview, error) {
	return application.CodexConfigurationPreview{
		Path: "C:\\test\\config.toml", Exists: true, Configured: true,
		Changes: map[string]string{"otel.log_user_prompt": "false"}, Restart: true,
	}, nil
}

func (testCodexConfiguration) Apply() (application.CodexConfigurationInfo, string, error) {
	return application.CodexConfigurationInfo{Configured: true}, "config.toml.backup.test", nil
}

func (testCodexConfiguration) Restore(string) (application.CodexConfigurationInfo, string, error) {
	return application.CodexConfigurationInfo{Configured: true}, "config.toml.backup.recovery", nil
}
