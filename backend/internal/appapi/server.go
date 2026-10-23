package appapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/application"
	"github.com/agentpulse/agentpulse/backend/internal/process"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

// Server hosts the authenticated application API on an ephemeral loopback port.
type Server struct {
	listener           net.Listener
	http               *http.Server
	token              string
	process            *application.ProcessService
	sessionQueries     *application.SessionQueryService
	sessionAnalytics   *application.SessionAnalyticsService
	trends             *application.AnalyticsTrendService
	retention          *application.TelemetryRetentionService
	ingestDiagnostics  *application.IngestDiagnosticService
	telemetryOverview  *application.TelemetryOverviewService
	telemetryData      *application.TelemetryDataService
	receiverSettings   *application.ReceiverSettingsService
	codexConfiguration application.CodexConfiguration
	ingestQueue        IngestQueueStatus
	backendVersion     string
	receiverAddress    string
	checkReceiverPort  func(int) error
	logCleaner         ApplicationLogCleaner
	changes            *telemetry.ChangeBus
	requestLimiter     *requestRateLimiter
}

type IngestQueueStatus interface {
	Depth() int
	Capacity() int
	Rejected() uint64
}

type ApplicationLogCleaner interface {
	CleanupExpired(context.Context) (int64, error)
}

// Dependencies are assembled by the executable composition root. HTTP
// transport receives application use cases and never creates storage adapters.
type Dependencies struct {
	Process            *application.ProcessService
	SessionQueries     *application.SessionQueryService
	SessionAnalytics   *application.SessionAnalyticsService
	Trends             *application.AnalyticsTrendService
	Retention          *application.TelemetryRetentionService
	IngestDiagnostics  *application.IngestDiagnosticService
	TelemetryOverview  *application.TelemetryOverviewService
	TelemetryData      *application.TelemetryDataService
	CodexConfiguration application.CodexConfiguration
	IngestQueue        IngestQueueStatus
	BackendVersion     string
	ReceiverAddress    string
	ReceiverSettings   *application.ReceiverSettingsService
	CheckReceiverPort  func(int) error
	LogCleaner         ApplicationLogCleaner
	Changes            *telemetry.ChangeBus
}

func Start(dependencies Dependencies) (*Server, error) {
	for _, required := range []struct {
		name    string
		missing bool
	}{
		{"process service", dependencies.Process == nil},
		{"session query service", dependencies.SessionQueries == nil},
		{"session analytics service", dependencies.SessionAnalytics == nil},
		{"analytics trends service", dependencies.Trends == nil},
		{"telemetry retention service", dependencies.Retention == nil},
		{"ingest diagnostics service", dependencies.IngestDiagnostics == nil},
		{"telemetry overview service", dependencies.TelemetryOverview == nil},
		{"telemetry data service", dependencies.TelemetryData == nil},
		{"receiver settings service", dependencies.ReceiverSettings == nil},
		{"receiver port availability checker", dependencies.CheckReceiverPort == nil},
		{"Codex configuration", dependencies.CodexConfiguration == nil},
		{"receiver address", dependencies.ReceiverAddress == ""},
	} {
		if required.missing {
			return nil, fmt.Errorf("missing application dependency: %s", required.name)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		listener.Close()
		return nil, err
	}
	if dependencies.Changes == nil {
		dependencies.Changes = telemetry.NewChangeBus()
	}
	if dependencies.BackendVersion == "" {
		dependencies.BackendVersion = "development"
	}
	s := &Server{
		listener: listener, token: base64.RawURLEncoding.EncodeToString(secret),
		process: dependencies.Process, sessionQueries: dependencies.SessionQueries,
		sessionAnalytics: dependencies.SessionAnalytics, trends: dependencies.Trends,
		retention: dependencies.Retention, ingestDiagnostics: dependencies.IngestDiagnostics,
		telemetryOverview: dependencies.TelemetryOverview, telemetryData: dependencies.TelemetryData,
		receiverSettings:   dependencies.ReceiverSettings,
		codexConfiguration: dependencies.CodexConfiguration,
		ingestQueue:        dependencies.IngestQueue, backendVersion: dependencies.BackendVersion,
		receiverAddress:   dependencies.ReceiverAddress,
		checkReceiverPort: dependencies.CheckReceiverPort,
		logCleaner:        dependencies.LogCleaner, changes: dependencies.Changes,
		requestLimiter: newRequestRateLimiter(
			apiRequestsPerSecond, apiRequestBurst, time.Now,
		),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", s.authorize(s.status))
	mux.HandleFunc("GET /api/v1/diagnostics/ingest", s.authorize(s.ingestDiagnosticsStatus))
	mux.HandleFunc("GET /api/v1/events", s.authorize(s.eventStream))
	mux.HandleFunc("GET /api/v1/dashboard/summary", s.authorize(s.summary))
	mux.HandleFunc("GET /api/v1/analytics/trends", s.authorize(s.analyticsTrends))
	mux.HandleFunc("GET /api/v1/sessions", s.authorize(s.sessions))
	mux.HandleFunc("GET /api/v1/sessions/{id}/timeline", s.authorize(s.sessionTimeline))
	mux.HandleFunc("GET /api/v1/sessions/{id}/analysis", s.authorize(s.sessionAnalysis))
	mux.HandleFunc("GET /api/v1/sessions/{id}/processes", s.authorize(s.sessionProcesses))
	mux.HandleFunc("GET /api/v1/events/unassigned", s.authorize(s.unassignedEvents))
	mux.HandleFunc("GET /api/v1/tools/summary", s.authorize(s.toolSummary))
	mux.HandleFunc("GET /api/v1/tools/calls", s.authorize(s.toolCalls))
	mux.HandleFunc("GET /api/v1/processes", s.authorize(s.processes))
	mux.HandleFunc("GET /api/v1/settings/process-monitor", s.authorize(s.processMonitorSettings))
	mux.HandleFunc("PUT /api/v1/settings/process-monitor", s.authorize(s.updateProcessMonitorSettings))
	mux.HandleFunc("GET /api/v1/settings/data-retention", s.authorize(s.dataRetentionSettings))
	mux.HandleFunc("PUT /api/v1/settings/data-retention", s.authorize(s.updateDataRetentionSettings))
	mux.HandleFunc("GET /api/v1/settings/receiver", s.authorize(s.receiverSettingsSnapshot))
	mux.HandleFunc("PUT /api/v1/settings/receiver", s.authorize(s.updateReceiverSettings))
	mux.HandleFunc("POST /api/v1/settings/data-retention/cleanup", s.authorize(s.requestDataCleanup))
	mux.HandleFunc("POST /api/v1/settings/logs/cleanup", s.authorize(s.cleanupApplicationLogs))
	mux.HandleFunc("GET /api/v1/data/export", s.authorize(s.exportData))
	mux.HandleFunc("DELETE /api/v1/data/history", s.authorize(s.clearHistory))
	mux.HandleFunc("GET /api/v1/config/codex", s.authorize(s.codexConfig))
	mux.HandleFunc("GET /api/v1/config/codex/preview", s.authorize(s.codexConfigPreview))
	mux.HandleFunc("POST /api/v1/config/codex/apply", s.authorize(s.codexConfigApply))
	mux.HandleFunc("POST /api/v1/config/codex/restore", s.authorize(s.codexConfigRestore))
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 30 * time.Second}
	go func() { _ = s.http.Serve(listener) }()
	return s, nil
}

// StartRetention starts the cleanup schedule after the owning process creates
// its shutdown context.
func (s *Server) StartRetention(ctx context.Context) { s.retention.Start(ctx) }

func (s *Server) WaitRetention() { s.retention.Wait() }

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	flusher, okay := w.(http.Flusher)
	if !okay {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "event_stream_unavailable"})
		return
	}
	updates, unsubscribe := s.changes.Subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case update := <-updates:
			payload, err := json.Marshal(update)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) ingestDiagnosticsStatus(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.ingestDiagnostics.Snapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest_diagnostics_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	limit := 25
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "all"
	}
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if len(model) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_model_filter"})
		return
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	var beforeActivity *int64
	var beforeID string
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		var value struct {
			LastActivityNS int64  `json:"last_activity_ns"`
			ID             string `json:"id"`
		}
		if err := json.Unmarshal(decoded, &value); err != nil || value.LastActivityNS <= 0 || value.ID == "" || len(value.ID) > 256 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		beforeActivity = &value.LastActivityNS
		beforeID = value.ID
	}
	items, err := s.sessionQueries.ListSessions(r.Context(), application.SessionListQuery{
		Range: rangeName, Model: model, Limit: limit + 1,
		BeforeAtNS: beforeActivity, BeforeID: beforeID,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidSessionQueryRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		if errors.Is(err, application.ErrInvalidSessionModel) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_model_filter"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session_query_unavailable"})
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor any
	if hasMore {
		last := items[len(items)-1]
		encoded, err := json.Marshal(struct {
			LastActivityNS int64  `json:"last_activity_ns"`
			ID             string `json:"id"`
		}{LastActivityNS: *last.LastActivityNS, ID: last.ID})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cursor_encode_failed"})
			return
		}
		nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"range": rangeName, "model": optionalString(model),
		"items": items, "limit": limit, "next_cursor": nextCursor,
	})
}

func (s *Server) sessionTimeline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	items, err := s.sessionQueries.Timeline(r.Context(), id, limit)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "timeline_query_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "items": items, "limit": limit, "data_quality": "observed_identifier"})
}

func (s *Server) sessionAnalysis(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	view, err := s.sessionAnalytics.Analyze(r.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrSessionAnalyticsNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session_not_found"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session_analysis_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) sessionProcesses(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	items, err := s.process.ListForSession(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session_process_query_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "items": items})
}

func (s *Server) unassignedEvents(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	var beforeID int64
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) > 128 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		var value struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(decoded, &value); err != nil || value.ID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		beforeID = value.ID
	}
	items, err := s.sessionQueries.UnassignedEvents(r.Context(), limit+1, beforeID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event_query_unavailable"})
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor any
	if hasMore {
		encoded, err := json.Marshal(struct {
			ID int64 `json:"id"`
		}{ID: items[len(items)-1].EventID})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cursor_encode_failed"})
			return
		}
		nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "next_cursor": nextCursor})
}

func (s *Server) toolSummary(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "24h"
	}
	sessionID := r.URL.Query().Get("session_id")
	if len(sessionID) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}
	summaries, err := s.sessionQueries.ToolSummary(r.Context(), rangeName, sessionID)
	if err != nil {
		if errors.Is(err, application.ErrInvalidSessionQueryRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tool_summary_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"range": rangeName, "session_id": optionalString(sessionID),
		"items": summaries, "rule_version": "tool-summary.v1",
	})
}

func (s *Server) toolCalls(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "24h"
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	sessionID := r.URL.Query().Get("session_id")
	toolName := r.URL.Query().Get("tool_name")
	if len(sessionID) > 256 || len(toolName) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_filter"})
		return
	}
	outcome := r.URL.Query().Get("outcome")
	var beforeAtNS *int64
	var beforeID string
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		var value struct {
			StartedAtNS int64  `json:"started_at_ns"`
			ID          string `json:"id"`
		}
		if err := json.Unmarshal(decoded, &value); err != nil || value.StartedAtNS <= 0 || value.ID == "" || len(value.ID) > 256 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		beforeAtNS = &value.StartedAtNS
		beforeID = value.ID
	}
	items, err := s.sessionQueries.ToolCalls(r.Context(), application.ToolCallQuery{
		Range: rangeName, SessionID: sessionID, ToolName: toolName, Outcome: outcome,
		Limit: limit + 1, BeforeAtNS: beforeAtNS, BeforeID: beforeID,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidSessionQueryRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
			return
		}
		if errors.Is(err, application.ErrInvalidToolOutcome) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_outcome"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tool_calls_unavailable"})
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor any
	if hasMore {
		last := items[len(items)-1]
		if last.StartedAtNS == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tool_call_cursor_unavailable"})
			return
		}
		encoded, err := json.Marshal(struct {
			StartedAtNS int64  `json:"started_at_ns"`
			ID          string `json:"id"`
		}{StartedAtNS: *last.StartedAtNS, ID: last.ID})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cursor_encode_failed"})
			return
		}
		nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"range": rangeName, "session_id": optionalString(sessionID), "tool_name": optionalString(toolName),
		"outcome": optionalString(outcome), "items": items, "limit": limit, "next_cursor": nextCursor,
	})
}

func (s *Server) processes(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
			return
		}
		limit = parsed
	}
	var beforeAtNS *int64
	var beforeID string
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		var value struct {
			OrderAtNS int64  `json:"order_at_ns"`
			ID        string `json:"id"`
		}
		if err := json.Unmarshal(decoded, &value); err != nil || value.OrderAtNS <= 0 || value.ID == "" || len(value.ID) > 128 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		beforeAtNS = &value.OrderAtNS
		beforeID = value.ID
	}
	items, err := s.process.List(r.Context(), limit+1, beforeAtNS, beforeID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "process_query_unavailable"})
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor any
	if hasMore {
		last := items[len(items)-1]
		encoded, err := json.Marshal(struct {
			OrderAtNS int64  `json:"order_at_ns"`
			ID        string `json:"id"`
		}{OrderAtNS: last.OrderAtNS, ID: last.ProcessID})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cursor_encode_failed"})
			return
		}
		nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "next_cursor": nextCursor})
}

func (s *Server) processMonitorSettings(w http.ResponseWriter, r *http.Request) {
	config, err := s.process.LoadSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "process_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": config, "status": s.process.Status()})
}

func (s *Server) updateProcessMonitorSettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	var config process.Config
	if err := decoder.Decode(&config); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_process_settings"})
		return
	}
	if _, err := decoder.Token(); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_process_settings"})
		return
	}
	validated, err := s.process.UpdateSettings(r.Context(), config)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidProcessSettings):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_process_settings", "message": err.Error()})
		case errors.Is(err, application.ErrProcessSettingsPersistence):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "process_settings_unavailable"})
		case errors.Is(err, application.ErrProcessMonitorUpdate):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "process_monitor_update_failed"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_process_settings", "message": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": validated, "status": s.process.Status()})
}

func (s *Server) dataRetentionSettings(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.retention.Snapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retention_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) receiverSettingsSnapshot(w http.ResponseWriter, r *http.Request) {
	settings, err := s.receiverSettings.Load(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "receiver_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) updateReceiverSettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var request struct {
		Port int `json:"port"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_receiver_settings"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_receiver_settings"})
		return
	}
	if err := application.ValidateReceiverPort(request.Port); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_receiver_settings", "message": err.Error()})
		return
	}
	current, err := s.receiverSettings.Load(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "receiver_settings_unavailable"})
		return
	}
	if request.Port != current.Port {
		if err := s.checkReceiverPort(request.Port); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "receiver_port_unavailable", "message": "所选端口当前不可用，请选择未被占用的端口。"})
			return
		}
	}
	settings, restartRequired, err := s.receiverSettings.Update(r.Context(), request.Port)
	if errors.Is(err, application.ErrInvalidReceiverPort) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_receiver_settings", "message": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "receiver_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings, "restart_required": restartRequired})
}

func (s *Server) updateDataRetentionSettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var request struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_retention_settings"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_retention_settings"})
		return
	}
	snapshot, err := s.retention.Update(r.Context(), request.RetentionDays)
	if err != nil {
		if errors.Is(err, application.ErrInvalidTelemetryRetention) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_retention_settings", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retention_settings_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) requestDataCleanup(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.retention.Snapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retention_settings_unavailable"})
		return
	}
	s.retention.RequestCleanup()
	writeJSON(w, http.StatusAccepted, map[string]any{"cleanup_scheduled": true, "retention": snapshot})
}

func (s *Server) cleanupApplicationLogs(w http.ResponseWriter, r *http.Request) {
	if s.logCleaner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "application_log_cleanup_unavailable"})
		return
	}
	deletedBytes, err := s.logCleaner.CleanupExpired(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "application_log_cleanup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_bytes": deletedBytes, "retention_days": 14})
}

func (s *Server) exportData(w http.ResponseWriter, r *http.Request) {
	snapshot, size, err := s.telemetryData.ExportSnapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database_export_unavailable"})
		return
	}
	defer snapshot.Close()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="agentpulse-data.db"`)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, snapshot)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.telemetryData.ClearHistory(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history_clear_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted_rows": deleted})
}

func (s *Server) processMonitorStatus() process.MonitorStatus {
	return s.process.Status()
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Server) codexConfig(w http.ResponseWriter, r *http.Request) {
	info, err := s.codexConfiguration.Detect()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "config_unavailable", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) codexConfigPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := s.codexConfiguration.Preview()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "config_preview_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) codexConfigApply(w http.ResponseWriter, r *http.Request) {
	info, backup, err := s.codexConfiguration.Apply()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "config_apply_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": info, "backup": backup, "restart_required": backup != ""})
}

func (s *Server) codexConfigRestore(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decoder.DisallowUnknownFields()
	var request struct {
		Backup string `json:"backup"`
	}
	if err := decoder.Decode(&request); err != nil || request.Backup == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_backup_request"})
		return
	}
	info, recovery, err := s.codexConfiguration.Restore(request.Backup)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "config_restore_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": info, "recovery_backup": recovery, "restart_required": true})
}

func (s *Server) Address() string { return s.listener.Addr().String() }
func (s *Server) Token() string   { return s.token }
func (s *Server) Close() error    { return s.http.Close() }

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed == nil {
				http.Error(w, `{"error":"origin_not_allowed"}`, http.StatusForbidden)
				return
			}
			host := parsed.Hostname()
			ip := net.ParseIP(host)
			if parsed.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
				http.Error(w, `{"error":"origin_not_allowed"}`, http.StatusForbidden)
				return
			}
		}
		if !s.requestLimiter.Allow() {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "api_rate_limit_exceeded"})
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	health, err := s.telemetryOverview.Status(r.Context(), time.Now())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "status_unavailable"})
		return
	}
	result := map[string]any{
		"api_version": 1, "backend_version": s.backendVersion,
		"schema_version": health.SchemaVersion, "receiver": "ready",
		"receiver_address": s.receiverAddress,
		"signals":          health.Signals, "process_monitor": s.processMonitorStatus(),
	}
	if diagnostics, err := s.ingestDiagnostics.Snapshot(r.Context()); err == nil {
		result["ingest_diagnostics"] = diagnostics
	}
	if s.ingestQueue != nil {
		result["ingest_queue"] = map[string]any{
			"depth": s.ingestQueue.Depth(), "capacity": s.ingestQueue.Capacity(),
			"rejected": s.ingestQueue.Rejected(),
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "1h"
	}
	result, err := s.telemetryOverview.Summary(r.Context(), rangeName, time.Now())
	if errors.Is(err, application.ErrInvalidTelemetrySummaryRange) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) analyticsTrends(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "1h"
	}
	view, err := s.trends.Query(r.Context(), rangeName)
	if errors.Is(err, application.ErrInvalidSessionQueryRange) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics_trends_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
