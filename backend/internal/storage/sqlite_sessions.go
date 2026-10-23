package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/agentpulse/agentpulse/backend/internal/correlation"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	"sort"
	"time"
)

func (s *SQLite) ListSessions(ctx context.Context, limit int, beforeActivityNS *int64, beforeID string, sinceNS *int64, model string) ([]telemetry.SessionSummary, error) {
	var cursorTime any
	if beforeActivityNS != nil {
		cursorTime = *beforeActivityNS
	}
	var sinceTime any
	if sinceNS != nil {
		sinceTime = *sinceNS
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.session_id,s.source_id,s.external_id,s.started_at_ns,s.last_activity_ns,s.model,s.event_count,
       (SELECT COUNT(*) FROM tool_calls tc WHERE tc.session_id=s.session_id),
       (SELECT COUNT(*) FROM tool_calls tc WHERE tc.session_id=s.session_id AND lower(tc.tool_name) IN ('shell','shell_command','exec','exec_command','command','run_command','terminal')),
       (SELECT COUNT(*) FROM tool_calls tc WHERE tc.session_id=s.session_id AND lower(COALESCE(tc.status,'')) IN ('error','failed','failure','cancelled')),
       s.input_tokens,s.output_tokens,s.reasoning_tokens,s.data_quality
FROM sessions s
WHERE (? IS NULL OR s.last_activity_ns >= ?)
AND (? = '' OR instr(lower(COALESCE(s.model,'')), lower(?)) > 0)
AND (? IS NULL OR s.last_activity_ns < ? OR (s.last_activity_ns = ? AND s.session_id < ?))
ORDER BY s.last_activity_ns DESC,s.session_id DESC LIMIT ?`, sinceTime, sinceTime, model, model, cursorTime, cursorTime, cursorTime, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.SessionSummary, 0, limit)
	for rows.Next() {
		var item telemetry.SessionSummary
		var started, last sql.NullInt64
		var model, quality sql.NullString
		var inputTokens, outputTokens, reasoningTokens sql.NullInt64
		if err := rows.Scan(&item.ID, &item.SourceID, &item.ExternalID, &started, &last, &model, &item.EventCount,
			&item.ToolCallCount, &item.CommandCount, &item.FailedCallCount,
			&inputTokens, &outputTokens, &reasoningTokens, &quality); err != nil {
			return nil, err
		}
		item.StartedAtNS = nullableInt64(started)
		item.LastActivityNS = nullableInt64(last)
		item.Model = model.String
		item.DataQuality = quality.String
		item.InputTokens = nullableInt64(inputTokens)
		item.OutputTokens = nullableInt64(outputTokens)
		item.ReasoningTokens = nullableInt64(reasoningTokens)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) LoadSessionAnalytics(ctx context.Context, sessionID string) (telemetry.SessionAnalyticsInput, error) {
	var input telemetry.SessionAnalyticsInput
	var started, last sql.NullInt64
	var model, quality sql.NullString
	var inputTokens, outputTokens, reasoningTokens sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT session_id,source_id,external_id,started_at_ns,last_activity_ns,model,input_tokens,output_tokens,reasoning_tokens,data_quality
FROM sessions WHERE session_id=?`, sessionID).Scan(
		&input.SessionID, &input.SourceID, &input.ExternalID, &started, &last, &model,
		&inputTokens, &outputTokens, &reasoningTokens, &quality)
	if err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}
	input.StartedAtNS = nullableInt64(started)
	input.LastActivityNS = nullableInt64(last)
	input.InputTokens = nullableInt64(inputTokens)
	input.OutputTokens = nullableInt64(outputTokens)
	input.ReasoningTokens = nullableInt64(reasoningTokens)
	input.DataQuality = quality.String

	rows, err := s.db.QueryContext(ctx, `
SELECT s.start_time_ns,s.end_time_ns,s.span_role,s.model,tc.tool_name
FROM spans s LEFT JOIN tool_calls tc ON tc.trace_id=s.trace_id AND tc.span_id=s.span_id
WHERE s.session_id=? ORDER BY s.start_time_ns,s.span_id`, sessionID)
	if err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}
	for rows.Next() {
		var startAt, endAt int64
		var role string
		var model, toolName sql.NullString
		if err := rows.Scan(&startAt, &endAt, &role, &model, &toolName); err != nil {
			rows.Close()
			return telemetry.SessionAnalyticsInput{}, err
		}
		if startAt <= 0 || endAt <= startAt {
			continue
		}
		interval := telemetry.TimeInterval{StartNS: startAt, EndNS: endAt}
		if role == "tool" {
			if toolName.Valid && correlation.IsCommandTool(toolName.String) {
				input.CommandIntervals = append(input.CommandIntervals, interval)
			}
			continue
		}
		if role == "model" || model.Valid {
			input.LLMIntervals = append(input.LLMIntervals, interval)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return telemetry.SessionAnalyticsInput{}, err
	}
	if err := rows.Close(); err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
SELECT AVG(ttft_ms),COUNT(*)
FROM telemetry_events
WHERE session_id=? AND signal_type='logs' AND ttft_ms IS NOT NULL`, sessionID).Scan(&input.TTFTMS, &input.TTFTSamples); err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}

	eventRows, err := s.db.QueryContext(ctx, `SELECT id FROM telemetry_events WHERE session_id=? ORDER BY id LIMIT 501`, sessionID)
	if err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}
	for eventRows.Next() {
		var id int64
		if err := eventRows.Scan(&id); err != nil {
			eventRows.Close()
			return telemetry.SessionAnalyticsInput{}, err
		}
		if len(input.SourceEventIDs) == 500 {
			input.SourceEventIDsTruncated = true
			break
		}
		input.SourceEventIDs = append(input.SourceEventIDs, id)
	}
	if err := eventRows.Close(); err != nil {
		return telemetry.SessionAnalyticsInput{}, err
	}
	return input, eventRows.Err()
}

func (s *SQLite) SessionTimeline(ctx context.Context, sessionID string, limit int) ([]telemetry.TimelineRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id,source_id,session_id,turn_id,signal_type,event_time_ns,received_at_ns,event_name,data_quality,attributes_json
FROM telemetry_events
WHERE session_id=?
  AND NOT (signal_type='traces' AND event_name='span' AND EXISTS (
    SELECT 1 FROM spans
    WHERE spans.source_id=telemetry_events.source_id
      AND spans.trace_id=telemetry_events.trace_id
      AND spans.span_id=telemetry_events.span_id
      AND spans.session_id=telemetry_events.session_id
  ))
ORDER BY COALESCE(event_time_ns,received_at_ns) ASC,id ASC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.TimelineRecord, 0, limit*2)
	for rows.Next() {
		var item telemetry.TimelineRecord
		var eventTime sql.NullInt64
		var sessionID, turnID sql.NullString
		var raw string
		if err := rows.Scan(&item.ID, &item.SourceID, &sessionID, &turnID, &item.Signal, &eventTime, &item.ReceivedAtNS, &item.Name, &item.DataQuality, &raw); err != nil {
			return nil, err
		}
		item.RecordType = "event"
		item.EventTimeNS = nullableInt64(eventTime)
		item.StartTimeNS = nullableInt64(eventTime)
		item.SessionID = nullableStringPointer(sessionID)
		item.TurnID = nullableStringPointer(turnID)
		if err := json.Unmarshal([]byte(raw), &item.Attributes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	spanRows, err := s.db.QueryContext(ctx, `
SELECT s.source_id,s.session_id,s.turn_id,s.span_id,s.parent_span_id,s.start_time_ns,s.end_time_ns,
       s.received_at_ns,s.span_role,s.model,s.process_pid,s.parent_process_pid,s.correlation_method,
       COALESCE(tc.data_quality,'partial'),s.attributes_json,tc.tool_call_id,tc.tool_name,tc.status,tc.retry_index,tc.duration_ms
FROM spans s
LEFT JOIN tool_calls tc ON tc.trace_id=s.trace_id AND tc.span_id=s.span_id
WHERE s.session_id=?
ORDER BY s.start_time_ns ASC,s.span_id ASC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	for spanRows.Next() {
		var item telemetry.TimelineRecord
		var storedSession, storedTurn, parentSpan, spanRole, model, correlationMethod, dataQuality, toolCallID, toolName, toolStatus sql.NullString
		var startAt, endAt, processPID, parentPID, retryIndex sql.NullInt64
		var receivedAt int64
		var duration sql.NullFloat64
		var raw string
		if err := spanRows.Scan(&item.SourceID, &storedSession, &storedTurn, &item.SpanID, &parentSpan,
			&startAt, &endAt, &receivedAt, &spanRole, &model, &processPID, &parentPID,
			&correlationMethod, &dataQuality, &raw, &toolCallID, &toolName, &toolStatus, &retryIndex, &duration); err != nil {
			spanRows.Close()
			return nil, err
		}
		item.RecordType = "span"
		item.Signal = telemetry.SignalTraces
		item.Name = "span"
		item.SpanRole = spanRole.String
		item.CorrelationMethod = correlationMethod.String
		item.DataQuality = dataQuality.String
		item.SessionID = nullableStringPointer(storedSession)
		item.TurnID = nullableStringPointer(storedTurn)
		item.ParentSpanID = parentSpan.String
		item.Model = model.String
		item.EventTimeNS = nullableInt64(startAt)
		item.StartTimeNS = nullableInt64(startAt)
		item.ReceivedAtNS = receivedAt
		if endAt.Valid && startAt.Valid && endAt.Int64 > startAt.Int64 {
			item.EndTimeNS = nullableInt64(endAt)
		} else if endAt.Valid {
			item.EndTimeNS = nullableInt64(endAt)
		}
		item.ProcessPID = nullableInt64(processPID)
		item.ParentProcessPID = nullableInt64(parentPID)
		item.ToolCallID = nullableStringPointer(toolCallID)
		item.ToolName = toolName.String
		item.ToolStatus = nullableStringPointer(toolStatus)
		item.RetryIndex = nullableInt64(retryIndex)
		item.DurationMS = nullableFloat64(duration)
		if item.DurationMS == nil && endAt.Valid && startAt.Valid && endAt.Int64 > startAt.Int64 {
			value := float64(endAt.Int64-startAt.Int64) / 1e6
			item.DurationMS = &value
		}
		if err := json.Unmarshal([]byte(raw), &item.Attributes); err != nil {
			spanRows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := spanRows.Err(); err != nil {
		spanRows.Close()
		return nil, err
	}
	if err := spanRows.Close(); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(left, right int) bool {
		leftAt := timelineOrderAt(items[left])
		rightAt := timelineOrderAt(items[right])
		if leftAt != rightAt {
			return leftAt < rightAt
		}
		if items[left].RecordType != items[right].RecordType {
			return items[left].RecordType < items[right].RecordType
		}
		if items[left].RecordType == "span" {
			return items[left].SpanID < items[right].SpanID
		}
		return items[left].ID < items[right].ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func timelineOrderAt(record telemetry.TimelineRecord) int64 {
	if record.EventTimeNS != nil && *record.EventTimeNS > 0 {
		return *record.EventTimeNS
	}
	return record.ReceivedAtNS
}

// ListUnassignedEvents returns telemetry events that cannot be reliably tied to
// an observed session. The cursor is the last event row ID from the previous
// page; zero starts at the newest event.
func (s *SQLite) ListUnassignedEvents(ctx context.Context, limit int, beforeID int64) ([]telemetry.TimelineRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id,source_id,session_id,turn_id,signal_type,event_time_ns,received_at_ns,event_name,data_quality,attributes_json
FROM telemetry_events
WHERE session_id IS NULL AND (? <= 0 OR id < ?)
ORDER BY id DESC LIMIT ?`, beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.TimelineRecord, 0, limit)
	for rows.Next() {
		var item telemetry.TimelineRecord
		var eventTime sql.NullInt64
		var sessionID, turnID sql.NullString
		var raw string
		if err := rows.Scan(&item.ID, &item.SourceID, &sessionID, &turnID, &item.Signal, &eventTime, &item.ReceivedAtNS, &item.Name, &item.DataQuality, &raw); err != nil {
			return nil, err
		}
		item.EventTimeNS = nullableInt64(eventTime)
		item.SessionID = nullableStringPointer(sessionID)
		item.TurnID = nullableStringPointer(turnID)
		if err := json.Unmarshal([]byte(raw), &item.Attributes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) ListToolSummary(ctx context.Context, sinceNS, untilNS int64, sessionID string) ([]telemetry.ToolSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
WITH filtered AS (
  SELECT tool_name,status,duration_ms FROM tool_calls
  WHERE start_time_ns>=? AND start_time_ns<=? AND (?='' OR session_id=?) AND tool_name!=''
), outcomes AS (
  SELECT tool_name,COUNT(*) AS total_calls,
    SUM(CASE WHEN lower(COALESCE(status,'')) IN ('success','ok','completed') THEN 1 ELSE 0 END) AS success_calls,
    SUM(CASE WHEN lower(COALESCE(status,'')) IN ('error','failed','failure','cancelled') THEN 1 ELSE 0 END) AS failure_calls
  FROM filtered GROUP BY tool_name
), ranked_durations AS (
  SELECT tool_name,duration_ms,
    ROW_NUMBER() OVER(PARTITION BY tool_name ORDER BY duration_ms) AS sample_rank,
    COUNT(*) OVER(PARTITION BY tool_name) AS sample_count
  FROM filtered WHERE duration_ms IS NOT NULL AND duration_ms>=0
), duration_summary AS (
  SELECT tool_name,MAX(sample_count) AS duration_samples,
    MAX(CASE WHEN sample_rank=(sample_count*50+99)/100 THEN duration_ms END) AS p50_duration_ms,
    MAX(CASE WHEN sample_rank=(sample_count*95+99)/100 THEN duration_ms END) AS p95_duration_ms
  FROM ranked_durations GROUP BY tool_name
)
SELECT outcomes.tool_name,outcomes.total_calls,outcomes.success_calls,outcomes.failure_calls,
  outcomes.total_calls-outcomes.success_calls-outcomes.failure_calls AS unknown_outcomes,
  outcomes.success_calls+outcomes.failure_calls AS outcome_samples,
  CASE WHEN outcomes.success_calls+outcomes.failure_calls=0 THEN NULL
       ELSE CAST(outcomes.failure_calls AS REAL)/(outcomes.success_calls+outcomes.failure_calls) END AS failure_rate,
  COALESCE(duration_summary.duration_samples,0),duration_summary.p50_duration_ms,duration_summary.p95_duration_ms,
  CASE WHEN outcomes.success_calls+outcomes.failure_calls=outcomes.total_calls
         AND COALESCE(duration_summary.duration_samples,0)=outcomes.total_calls THEN 'complete' ELSE 'partial' END AS data_quality
FROM outcomes LEFT JOIN duration_summary USING(tool_name)
ORDER BY outcomes.total_calls DESC,outcomes.tool_name ASC`, sinceNS, untilNS, sessionID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.ToolSummary, 0)
	for rows.Next() {
		var item telemetry.ToolSummary
		var failureRate, p50, p95 sql.NullFloat64
		if err := rows.Scan(&item.ToolName, &item.TotalCalls, &item.SuccessCalls, &item.FailureCalls,
			&item.UnknownOutcomes, &item.OutcomeSamples, &failureRate, &item.DurationSamples, &p50, &p95, &item.DataQuality); err != nil {
			return nil, err
		}
		if failureRate.Valid {
			item.FailureRate = &failureRate.Float64
		}
		if p50.Valid {
			item.P50DurationMS = &p50.Float64
		}
		if p95.Valid {
			item.P95DurationMS = &p95.Float64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) ListToolCalls(ctx context.Context, sinceNS, untilNS int64, sessionID, toolName, outcome string, limit int, beforeAtNS *int64, beforeID string) ([]telemetry.ToolCallRecord, error) {
	var cursorAt any
	if beforeAtNS != nil {
		cursorAt = *beforeAtNS
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT tc.tool_call_id,tc.source_id,tc.external_tool_call_id,tc.session_id,tc.turn_id,
       tc.tool_name,tc.tool_type,tc.start_time_ns,tc.end_time_ns,tc.duration_ms,tc.status,
       tc.retry_index,tc.trace_id,tc.span_id,tc.source_event_id,tc.correlation_method,
       tc.correlation_confidence,tc.data_quality
FROM tool_calls tc
WHERE tc.start_time_ns>=? AND tc.start_time_ns<=?
  AND (?='' OR tc.session_id=?)
  AND (?='' OR tc.tool_name=?)
  AND (?='' OR (?='successful' AND lower(COALESCE(tc.status,'')) IN ('success','ok','completed'))
       OR (?='failed' AND lower(COALESCE(tc.status,'')) IN ('error','failed','failure','cancelled'))
       OR (?='unknown' AND lower(COALESCE(tc.status,'')) NOT IN ('success','ok','completed','error','failed','failure','cancelled')))
  AND (? IS NULL OR tc.start_time_ns<? OR (tc.start_time_ns=? AND tc.tool_call_id<?))
ORDER BY tc.start_time_ns DESC,tc.tool_call_id DESC
LIMIT ?`, sinceNS, untilNS, sessionID, sessionID, toolName, toolName,
		outcome, outcome, outcome, outcome, cursorAt, cursorAt, cursorAt, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]telemetry.ToolCallRecord, 0, limit)
	for rows.Next() {
		var item telemetry.ToolCallRecord
		var endedAt, retryIndex sql.NullInt64
		var duration sql.NullFloat64
		var externalIDText, sessionText, turnText, toolTypeText, statusText, traceID, spanID sql.NullString
		var startAt, eventID sql.NullInt64
		var confidence sql.NullFloat64
		if err := rows.Scan(&item.ID, &item.SourceID, &externalIDText, &sessionText, &turnText,
			&item.ToolName, &toolTypeText, &startAt, &endedAt, &duration, &statusText, &retryIndex,
			&traceID, &spanID, &eventID, &item.CorrelationMethod, &confidence, &item.DataQuality); err != nil {
			return nil, err
		}
		item.ExternalID = nullableStringPointer(externalIDText)
		item.SessionID = nullableStringPointer(sessionText)
		item.TurnID = nullableStringPointer(turnText)
		item.ToolType = nullableStringPointer(toolTypeText)
		item.StartedAtNS = nullableInt64(startAt)
		item.EndedAtNS = nullableInt64(endedAt)
		item.DurationMS = nullableFloat64(duration)
		item.Status = nullableStringPointer(statusText)
		item.RetryIndex = nullableInt64(retryIndex)
		item.TraceID = nullableStringPointer(traceID)
		item.SpanID = nullableStringPointer(spanID)
		item.SourceEventID = nullableInt64(eventID)
		if confidence.Valid {
			item.CorrelationConfidence = &confidence.Float64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLite) LoadAnalyticsTrends(ctx context.Context, sinceNS, untilNS, bucketWidthNS int64) ([]telemetry.AnalyticsTrendBucket, error) {
	if bucketWidthNS <= 0 || sinceNS <= 0 || untilNS < sinceNS {
		return nil, errors.New("invalid analytics trend window")
	}
	buckets := make(map[int64]telemetry.AnalyticsTrendBucket)
	rows, err := s.db.QueryContext(ctx, `
SELECT (event_time_ns / ?)*? AS bucket_start_ns,
       SUM(input_tokens),COUNT(input_tokens),SUM(output_tokens),COUNT(output_tokens),
       COUNT(ttft_ms),AVG(ttft_ms)
FROM telemetry_events
WHERE signal_type='logs' AND event_time_ns>=? AND event_time_ns<=?
GROUP BY bucket_start_ns`, bucketWidthNS, bucketWidthNS, sinceNS, untilNS)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var atNS int64
		var input, output sql.NullInt64
		var inputSamples, outputSamples, ttftSamples int64
		var averageTTFT sql.NullFloat64
		if err := rows.Scan(&atNS, &input, &inputSamples, &output, &outputSamples, &ttftSamples, &averageTTFT); err != nil {
			rows.Close()
			return nil, err
		}
		bucket := buckets[atNS]
		bucket.StartedAtNS = atNS
		bucket.HasData = true
		bucket.InputTokens = nullableInt64(input)
		bucket.InputTokenSamples = inputSamples
		bucket.OutputTokens = nullableInt64(output)
		bucket.OutputTokenSamples = outputSamples
		bucket.TTFTSamples = ttftSamples
		bucket.AverageTTFTMS = nullableFloat64(averageTTFT)
		buckets[atNS] = bucket
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, `
SELECT e.event_time_ns,e.output_tokens,s.end_time_ns-s.start_time_ns
FROM telemetry_events e
JOIN spans s ON s.trace_id=e.trace_id AND s.span_id=e.span_id
WHERE e.signal_type='logs' AND e.event_time_ns>=? AND e.event_time_ns<=?
  AND e.output_tokens IS NOT NULL AND s.span_role='model' AND s.end_time_ns>s.start_time_ns`, sinceNS, untilNS)
	if err != nil {
		return nil, err
	}
	rateSums := make(map[int64]float64)
	rateCounts := make(map[int64]int64)
	for rows.Next() {
		var eventAtNS, outputTokens, durationNS int64
		if err := rows.Scan(&eventAtNS, &outputTokens, &durationNS); err != nil {
			rows.Close()
			return nil, err
		}
		atNS := (eventAtNS / bucketWidthNS) * bucketWidthNS
		rateSums[atNS] += float64(outputTokens) / (float64(durationNS) / float64(time.Second))
		rateCounts[atNS]++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for atNS, totalRate := range rateSums {
		bucket := buckets[atNS]
		bucket.StartedAtNS = atNS
		bucket.HasData = true
		bucket.RequestRateSamples = rateCounts[atNS]
		averageRate := totalRate / float64(rateCounts[atNS])
		bucket.AverageRequestRateTPS = &averageRate
		buckets[atNS] = bucket
	}
	rows, err = s.db.QueryContext(ctx, `
SELECT (start_time_ns / ?)*? AS bucket_start_ns,COUNT(*),
       COUNT(CASE WHEN duration_ms>=0 THEN 1 END),AVG(CASE WHEN duration_ms>=0 THEN duration_ms END)
FROM tool_calls
WHERE start_time_ns>=? AND start_time_ns<=?
GROUP BY bucket_start_ns`, bucketWidthNS, bucketWidthNS, sinceNS, untilNS)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var atNS int64
		var callCount, durationSamples int64
		var averageDuration sql.NullFloat64
		if err := rows.Scan(&atNS, &callCount, &durationSamples, &averageDuration); err != nil {
			rows.Close()
			return nil, err
		}
		bucket := buckets[atNS]
		bucket.StartedAtNS = atNS
		bucket.HasData = true
		bucket.ToolCallCount = callCount
		bucket.DurationSamples = durationSamples
		bucket.AverageDurationMS = nullableFloat64(averageDuration)
		buckets[atNS] = bucket
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	firstBucket := (sinceNS / bucketWidthNS) * bucketWidthNS
	lastBucket := (untilNS / bucketWidthNS) * bucketWidthNS
	result := make([]telemetry.AnalyticsTrendBucket, 0, (lastBucket-firstBucket)/bucketWidthNS+1)
	for atNS := firstBucket; atNS <= lastBucket; atNS += bucketWidthNS {
		bucket := buckets[atNS]
		bucket.StartedAtNS = atNS
		result = append(result, bucket)
	}
	return result, nil
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func nullableFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int64Pointer(value int64) *int64 { return &value }
