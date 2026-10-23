package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/analytics"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

var ErrSessionAnalyticsNotFound = errors.New("session not found")

type SessionAnalyticsRepository interface {
	LoadSessionAnalytics(context.Context, string) (telemetry.SessionAnalyticsInput, error)
	ListToolSummary(context.Context, int64, int64, string) ([]telemetry.ToolSummary, error)
}

type SessionAnalyticsView struct {
	SessionID               string                     `json:"session_id"`
	SourceID                string                     `json:"source_id"`
	ExternalID              string                     `json:"external_id"`
	SessionStatus           string                     `json:"session_status"`
	ActivityWindowStartNS   *int64                     `json:"activity_window_start_ns"`
	ActivityWindowEndNS     *int64                     `json:"activity_window_end_ns"`
	ActivityWindowMS        *float64                   `json:"activity_window_ms"`
	ActivityWindowQuality   analytics.Quality          `json:"activity_window_quality"`
	InputTokens             *int64                     `json:"input_tokens"`
	OutputTokens            *int64                     `json:"output_tokens"`
	ReasoningTokens         *int64                     `json:"reasoning_tokens"`
	Durations               analytics.IntervalAnalysis `json:"durations"`
	Tokens                  analytics.TokenAnalysis    `json:"tokens"`
	Tools                   []telemetry.ToolSummary    `json:"tools"`
	ToolRuleVersion         string                     `json:"tool_rule_version"`
	DataQuality             string                     `json:"data_quality"`
	SourceEventIDs          []int64                    `json:"source_event_ids"`
	SourceEventIDsTruncated bool                       `json:"source_event_ids_truncated"`
}

type SessionAnalyticsService struct {
	repository SessionAnalyticsRepository
}

func NewSessionAnalyticsService(repository SessionAnalyticsRepository) *SessionAnalyticsService {
	return &SessionAnalyticsService{repository: repository}
}

func (s *SessionAnalyticsService) Analyze(ctx context.Context, sessionID string) (SessionAnalyticsView, error) {
	input, err := s.repository.LoadSessionAnalytics(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionAnalyticsView{}, fmt.Errorf("%w: %s", ErrSessionAnalyticsNotFound, sessionID)
	}
	if err != nil {
		return SessionAnalyticsView{}, err
	}

	var wallClock *analytics.Interval
	var activityWindowMS *float64
	if input.StartedAtNS != nil && input.LastActivityNS != nil && *input.LastActivityNS > *input.StartedAtNS {
		wallClock = &analytics.Interval{StartNS: *input.StartedAtNS, EndNS: *input.LastActivityNS}
		value := float64(*input.LastActivityNS-*input.StartedAtNS) / 1e6
		activityWindowMS = &value
	}
	llmIntervals := make([]analytics.Interval, 0, len(input.LLMIntervals))
	for _, interval := range input.LLMIntervals {
		llmIntervals = append(llmIntervals, analytics.Interval{StartNS: interval.StartNS, EndNS: interval.EndNS})
	}
	commandIntervals := make([]analytics.Interval, 0, len(input.CommandIntervals))
	for _, interval := range input.CommandIntervals {
		commandIntervals = append(commandIntervals, analytics.Interval{StartNS: interval.StartNS, EndNS: interval.EndNS})
	}
	durations := analytics.AnalyzeIntervals(analytics.IntervalInput{
		WallClock: wallClock, WallClockQuality: analytics.QualityPartial,
		LLM: llmIntervals, Commands: commandIntervals,
		LLMObserved: len(llmIntervals) > 0, CommandsObserved: len(commandIntervals) > 0,
	})

	var requestDurationNS *int64
	if len(input.LLMIntervals) == 1 {
		interval := input.LLMIntervals[0]
		if interval.EndNS > interval.StartNS {
			value := interval.EndNS - interval.StartNS
			requestDurationNS = &value
		}
	}
	ttftQuality := analytics.QualityUnavailable
	if input.TTFTSamples == 1 {
		ttftQuality = analytics.QualityComplete
	} else if input.TTFTSamples > 1 {
		ttftQuality = analytics.QualityPartial
	}
	tokens := analytics.AnalyzeTokens(analytics.TokenInput{
		OutputTokens: input.OutputTokens, RequestDurationNS: requestDurationNS,
		TTFTMS: input.TTFTMS, RateQuality: analytics.QualityPartial, TTFTQuality: ttftQuality,
	})

	tools, err := s.repository.ListToolSummary(ctx, 0, time.Now().UnixNano(), sessionID)
	if err != nil {
		return SessionAnalyticsView{}, err
	}
	dataQuality := "partial"
	return SessionAnalyticsView{
		SessionID: input.SessionID, SourceID: input.SourceID, ExternalID: input.ExternalID,
		SessionStatus: "unknown", ActivityWindowStartNS: input.StartedAtNS,
		ActivityWindowEndNS: input.LastActivityNS, ActivityWindowMS: activityWindowMS,
		ActivityWindowQuality: analytics.QualityPartial,
		InputTokens:           input.InputTokens, OutputTokens: input.OutputTokens, ReasoningTokens: input.ReasoningTokens,
		Durations: durations, Tokens: tokens,
		Tools: tools, ToolRuleVersion: "tool-summary.v1", DataQuality: dataQuality,
		SourceEventIDs: input.SourceEventIDs, SourceEventIDsTruncated: input.SourceEventIDsTruncated,
	}, nil
}
