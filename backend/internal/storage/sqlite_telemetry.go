package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agentpulse/agentpulse/backend/internal/correlation"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

func (s *SQLite) SaveTelemetry(ctx context.Context, batch telemetry.Batch) error {
	if len(batch.EventMetadata) != 0 && len(batch.EventMetadata) != len(batch.Events) {
		return fmt.Errorf("event metadata count does not match telemetry events")
	}
	if len(batch.SpanMetadata) != 0 && len(batch.SpanMetadata) != len(batch.Spans) {
		return fmt.Errorf("span metadata count does not match telemetry spans")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertObservedAttributeKeys(ctx, tx, batch.Signal, batch.ObservedAttributeKeys, batch.ReceivedAtNS); err != nil {
		return err
	}
	sourceID := strings.TrimSpace(batch.SourceID)
	if sourceID == "" {
		sourceID = telemetry.SourceCodex
	}
	for index, event := range batch.Events {
		eventID, err := insertEvent(ctx, tx, batch.Signal, batch.ReceivedAtNS, event)
		if err != nil {
			return err
		}
		if len(batch.EventMetadata) > 0 {
			if err := persistCanonicalEvent(ctx, tx, eventID, batch.ReceivedAtNS, event, batch.EventMetadata[index]); err != nil {
				return err
			}
		}
	}
	for index, span := range batch.Spans {
		encoded, err := json.Marshal(span.Attributes)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO spans(trace_id,span_id,parent_span_id,name,start_time_ns,end_time_ns,status_code,attributes_json,source_id,received_at_ns) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(trace_id,span_id) DO UPDATE SET parent_span_id=excluded.parent_span_id,name=excluded.name,start_time_ns=excluded.start_time_ns,end_time_ns=excluded.end_time_ns,status_code=excluded.status_code,attributes_json=excluded.attributes_json,source_id=excluded.source_id,received_at_ns=excluded.received_at_ns`, span.TraceID, span.SpanID, nullableString(span.ParentSpanID), "span", span.StartTimeNS, span.EndTimeNS, span.StatusCode, string(encoded), sourceID, batch.ReceivedAtNS); err != nil {
			return err
		}
		if len(batch.SpanMetadata) > 0 {
			if err := persistCanonicalSpan(ctx, tx, batch.ReceivedAtNS, span, batch.SpanMetadata[index]); err != nil {
				return err
			}
		}
	}
	for _, metric := range batch.Metrics {
		encoded, err := json.Marshal(metric.Attributes)
		if err != nil {
			return err
		}
		aggregation, err := json.Marshal(metric.Aggregation)
		if err != nil {
			return err
		}
		var monotonic any
		if metric.Monotonic != nil {
			monotonic = *metric.Monotonic
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO metric_points(metric_name,metric_type,time_ns,numeric_value,unit,attributes_json,received_at_ns,start_time_ns,temporality,monotonic,aggregation_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, safeName(metric.Name), metric.Type, metric.TimeNS, metric.Value, safeName(metric.Unit), string(encoded), batch.ReceivedAtNS, metric.StartTimeNS, metric.Temporality, monotonic, string(aggregation)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value,updated_at_ns) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at_ns=excluded.updated_at_ns`, "last_received_"+string(batch.Signal)+"_ns", strconv.FormatInt(batch.ReceivedAtNS, 10), batch.ReceivedAtNS); err != nil {
		return err
	}
	return tx.Commit()
}

func insertEvent(ctx context.Context, tx *sql.Tx, signal telemetry.Signal, received int64, event telemetry.Event) (int64, error) {
	encoded, err := json.Marshal(event.Attributes)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO telemetry_events(signal_type,event_time_ns,received_at_ns,trace_id,span_id,event_name,attributes_json) VALUES(?,?,?,?,?,?,?)`, string(signal), event.EventTimeNS, received, nullableString(event.TraceID), nullableString(event.SpanID), event.Name, string(encoded))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func inheritSpanAssociation(ctx context.Context, tx *sql.Tx, traceID, spanID string, metadata telemetry.CanonicalMetadata) (telemetry.CanonicalMetadata, error) {
	if traceID == "" || spanID == "" {
		return metadata, nil
	}
	var externalSessionID, sessionID, externalTurnID, turnID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT external_session_id,session_id,external_turn_id,turn_id
FROM spans WHERE source_id=? AND trace_id=? AND span_id=?`, metadata.SourceID, traceID, spanID).Scan(
		&externalSessionID, &sessionID, &externalTurnID, &turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return metadata, nil
	}
	if err != nil {
		return telemetry.CanonicalMetadata{}, err
	}
	spanMetadata := telemetry.CanonicalMetadata{SourceID: metadata.SourceID}
	if externalSessionID.Valid {
		spanMetadata.ExternalSessionID = externalSessionID.String
	}
	if sessionID.Valid {
		spanMetadata.SessionID = sessionID.String
	}
	if externalTurnID.Valid {
		spanMetadata.ExternalTurnID = externalTurnID.String
	}
	if turnID.Valid {
		spanMetadata.TurnID = turnID.String
	}
	merged, _ := correlation.MergeTraceSpan(metadata, spanMetadata)
	return merged, nil
}

func persistCanonicalEvent(ctx context.Context, tx *sql.Tx, eventID, receivedAtNS int64, event telemetry.Event, metadata telemetry.CanonicalMetadata) error {
	var err error
	metadata, err = inheritSpanAssociation(ctx, tx, event.TraceID, event.SpanID, metadata)
	if err != nil {
		return err
	}
	if event.EventTimeNS != nil && (metadata.SessionID != "" || metadata.TurnID != "") {
		metadata.DataQuality = "complete"
	}
	toolCallID := ""
	if call := metadata.ToolCall; call != nil {
		toolCallID = makeToolCallID(metadata.SourceID, metadata.SessionID, call.ExternalID, event.TraceID, event.SpanID, eventID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE telemetry_events SET source_id=?,external_session_id=?,session_id=?,external_turn_id=?,turn_id=?,tool_call_id=?,correlation_method=?,data_quality=?,ttft_ms=?,input_tokens=?,output_tokens=?,reasoning_tokens=? WHERE id=?`,
		metadata.SourceID, nullableString(metadata.ExternalSessionID), nullableString(metadata.SessionID), nullableString(metadata.ExternalTurnID), nullableString(metadata.TurnID), nullableString(toolCallID), metadata.CorrelationMethod, metadata.DataQuality,
		nullableFloat64Arg(metadata.TTFTMS), nullableInt64Arg(metadata.InputTokens), nullableInt64Arg(metadata.OutputTokens), nullableInt64Arg(metadata.ReasoningTokens), eventID); err != nil {
		return err
	}
	if metadata.ExternalSessionID != "" {
		if err := upsertSession(ctx, tx, metadata.SessionID, metadata.SourceID, metadata.ExternalSessionID, metadata.ActivityAtNS, metadata.Model, 1,
			nullableInt64Arg(metadata.InputTokens), nullableInt64Arg(metadata.OutputTokens), nullableInt64Arg(metadata.ReasoningTokens), metadata.DataQuality, receivedAtNS); err != nil {
			return err
		}
	}
	if metadata.ExternalTurnID != "" {
		if err := upsertTurn(ctx, tx, metadata.TurnID, metadata.SourceID, metadata.ExternalTurnID, metadata.SessionID, metadata.ActivityAtNS, metadata.Model,
			nullableInt64Arg(metadata.InputTokens), nullableInt64Arg(metadata.OutputTokens), nullableInt64Arg(metadata.ReasoningTokens), metadata.DataQuality); err != nil {
			return err
		}
	}
	if call := metadata.ToolCall; call != nil {
		if err := persistToolCall(ctx, tx, metadata.SourceID, metadata.SessionID, metadata.TurnID, call.ExternalID, call.Name,
			call.Type, call.StartTimeNS, call.EndTimeNS, call.DurationMS, call.Status, call.RetryIndex,
			call.TraceID, call.SpanID, eventID, metadata.CorrelationMethod, metadata.CorrelationConfidence); err != nil {
			return err
		}
	}
	return nil
}

func persistCanonicalSpan(ctx context.Context, tx *sql.Tx, receivedAtNS int64, span telemetry.Span, metadata telemetry.CanonicalMetadata) error {
	var confidence any
	if metadata.CorrelationConfidence != nil {
		confidence = *metadata.CorrelationConfidence
	}
	if _, err := tx.ExecContext(ctx, `UPDATE spans SET source_id=?,external_session_id=?,session_id=?,external_turn_id=?,turn_id=?,correlation_method=?,correlation_confidence=?,span_role=?,model=?,process_pid=?,parent_process_pid=? WHERE trace_id=? AND span_id=?`,
		metadata.SourceID, nullableString(metadata.ExternalSessionID), nullableString(metadata.SessionID), nullableString(metadata.ExternalTurnID), nullableString(metadata.TurnID), metadata.CorrelationMethod, confidence,
		metadata.SpanRole, nullableString(metadata.Model), nullableInt64Arg(metadata.ProcessPID), nullableInt64Arg(metadata.ParentProcessPID), span.TraceID, span.SpanID); err != nil {
		return err
	}
	if metadata.ExternalSessionID != "" {
		if err := upsertSession(ctx, tx, metadata.SessionID, metadata.SourceID, metadata.ExternalSessionID, metadata.ActivityAtNS, metadata.Model, 0, nil, nil, nil, metadata.DataQuality, receivedAtNS); err != nil {
			return err
		}
	}
	if metadata.ExternalTurnID != "" {
		if err := upsertTurn(ctx, tx, metadata.TurnID, metadata.SourceID, metadata.ExternalTurnID, metadata.SessionID, metadata.ActivityAtNS, metadata.Model, nil, nil, nil, metadata.DataQuality); err != nil {
			return err
		}
	}
	if err := reconcileTraceSpanEvents(ctx, tx, receivedAtNS, span, metadata); err != nil {
		return err
	}
	call := metadata.ToolCall
	if call == nil {
		return nil
	}
	var eventID sql.NullInt64
	if span.TraceID != "" && span.SpanID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT MAX(id) FROM telemetry_events WHERE signal_type='traces' AND trace_id=? AND span_id=?`, span.TraceID, span.SpanID).Scan(&eventID); err != nil {
			return err
		}
	}
	eventIDValue := int64(0)
	if eventID.Valid {
		eventIDValue = eventID.Int64
	}
	return persistToolCall(ctx, tx, metadata.SourceID, metadata.SessionID, metadata.TurnID,
		call.ExternalID, call.Name, call.Type, call.StartTimeNS, call.EndTimeNS, call.DurationMS, call.Status, call.RetryIndex,
		call.TraceID, call.SpanID, eventIDValue, metadata.CorrelationMethod, metadata.CorrelationConfidence)
}

type traceSpanEvent struct {
	id                int64
	eventAtNS         sql.NullInt64
	receivedAtNS      int64
	externalSessionID sql.NullString
	sessionID         sql.NullString
	externalTurnID    sql.NullString
	turnID            sql.NullString
	inputTokens       sql.NullInt64
	outputTokens      sql.NullInt64
	reasoningTokens   sql.NullInt64
	dataQuality       string
}

func reconcileTraceSpanEvents(ctx context.Context, tx *sql.Tx, receivedAtNS int64, span telemetry.Span, metadata telemetry.CanonicalMetadata) error {
	if span.TraceID == "" || span.SpanID == "" || (metadata.SessionID == "" && metadata.TurnID == "") {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,event_time_ns,received_at_ns,external_session_id,session_id,
external_turn_id,turn_id,input_tokens,output_tokens,reasoning_tokens,data_quality
FROM telemetry_events
WHERE source_id=? AND trace_id=? AND span_id=?
  AND ((session_id IS NULL AND ?!='') OR (turn_id IS NULL AND ?!=''))`,
		metadata.SourceID, span.TraceID, span.SpanID, metadata.SessionID, metadata.TurnID)
	if err != nil {
		return err
	}
	var events []traceSpanEvent
	for rows.Next() {
		var event traceSpanEvent
		if err := rows.Scan(&event.id, &event.eventAtNS, &event.receivedAtNS, &event.externalSessionID, &event.sessionID,
			&event.externalTurnID, &event.turnID, &event.inputTokens, &event.outputTokens, &event.reasoningTokens, &event.dataQuality); err != nil {
			rows.Close()
			return err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, event := range events {
		eventMetadata := telemetry.CanonicalMetadata{
			SourceID: metadata.SourceID, DataQuality: event.dataQuality, ActivityAtNS: nullableInt64(event.eventAtNS),
		}
		if event.externalSessionID.Valid {
			eventMetadata.ExternalSessionID = event.externalSessionID.String
		}
		if event.sessionID.Valid {
			eventMetadata.SessionID = event.sessionID.String
		}
		if event.externalTurnID.Valid {
			eventMetadata.ExternalTurnID = event.externalTurnID.String
		}
		if event.turnID.Valid {
			eventMetadata.TurnID = event.turnID.String
		}
		merged, inherited := correlation.MergeTraceSpan(eventMetadata, metadata)
		if !inherited {
			continue
		}
		if event.eventAtNS.Valid && (merged.SessionID != "" || merged.TurnID != "") {
			merged.DataQuality = "complete"
		}
		needsSession := !event.sessionID.Valid && merged.SessionID != ""
		needsTurn := !event.turnID.Valid && merged.TurnID != ""
		if !needsSession && !needsTurn {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE telemetry_events SET
external_session_id=CASE WHEN session_id IS NULL AND ?!='' THEN ? ELSE external_session_id END,
session_id=CASE WHEN session_id IS NULL AND ?!='' THEN ? ELSE session_id END,
external_turn_id=CASE WHEN turn_id IS NULL AND ?!='' THEN ? ELSE external_turn_id END,
turn_id=CASE WHEN turn_id IS NULL AND ?!='' THEN ? ELSE turn_id END,
correlation_method=CASE WHEN (session_id IS NULL AND ?!='') OR (turn_id IS NULL AND ?!='') THEN 'trace_span' ELSE correlation_method END,
data_quality=CASE WHEN event_time_ns IS NOT NULL AND
  (COALESCE(session_id,NULLIF(?,'')) IS NOT NULL OR COALESCE(turn_id,NULLIF(?,'')) IS NOT NULL)
  THEN 'complete' ELSE data_quality END
WHERE id=?`,
			merged.ExternalSessionID, merged.ExternalSessionID, merged.SessionID, merged.SessionID,
			merged.ExternalTurnID, merged.ExternalTurnID, merged.TurnID, merged.TurnID,
			merged.SessionID, merged.TurnID, merged.SessionID, merged.TurnID, event.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tool_calls SET
session_id=COALESCE(session_id,NULLIF(?,'')),
turn_id=COALESCE(turn_id,NULLIF(?,'')),
correlation_method='trace_span',correlation_confidence=1.0,
data_quality=CASE WHEN ? THEN 'complete' ELSE data_quality END
WHERE source_event_id=? AND source_id=? AND ((session_id IS NULL AND ?!='') OR (turn_id IS NULL AND ?!=''))`,
			merged.SessionID, merged.TurnID, event.eventAtNS.Valid, event.id, metadata.SourceID,
			merged.SessionID, merged.TurnID); err != nil {
			return err
		}

		activityAt := nullableInt64(event.eventAtNS)
		if activityAt == nil && event.receivedAtNS > 0 {
			activityAt = int64Pointer(event.receivedAtNS)
		}
		quality := event.dataQuality
		if merged.DataQuality == "complete" {
			quality = "complete"
		}
		inputTokens := nullableInt64Arg(nullableInt64(event.inputTokens))
		outputTokens := nullableInt64Arg(nullableInt64(event.outputTokens))
		reasoningTokens := nullableInt64Arg(nullableInt64(event.reasoningTokens))
		if needsSession {
			if err := upsertSession(ctx, tx, merged.SessionID, metadata.SourceID, merged.ExternalSessionID, activityAt,
				metadata.Model, 1, inputTokens, outputTokens, reasoningTokens, quality, receivedAtNS); err != nil {
				return err
			}
		}
		if merged.TurnID != "" && (needsTurn || needsSession && event.turnID.Valid) {
			var turnInput, turnOutput, turnReasoning any
			if needsTurn {
				turnInput, turnOutput, turnReasoning = inputTokens, outputTokens, reasoningTokens
			}
			if err := upsertTurn(ctx, tx, merged.TurnID, metadata.SourceID, merged.ExternalTurnID, merged.SessionID,
				activityAt, metadata.Model, turnInput, turnOutput, turnReasoning, quality); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertSession(ctx context.Context, tx *sql.Tx, sessionID, sourceID, externalID string, activityAt *int64, model string, eventDelta int64, inputTokens, outputTokens, reasoningTokens any, quality string, receivedAtNS int64) error {
	var timestamp any
	if activityAt != nil {
		timestamp = *activityAt
	}
	return execTx(ctx, tx, `INSERT INTO sessions(session_id,source_id,external_id,started_at_ns,last_activity_ns,model,event_count,input_tokens,output_tokens,reasoning_tokens,data_quality,created_at_ns)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(session_id) DO UPDATE SET
  started_at_ns=CASE WHEN sessions.started_at_ns IS NULL THEN excluded.started_at_ns WHEN excluded.started_at_ns IS NULL THEN sessions.started_at_ns ELSE MIN(sessions.started_at_ns,excluded.started_at_ns) END,
  last_activity_ns=CASE WHEN sessions.last_activity_ns IS NULL THEN excluded.last_activity_ns WHEN excluded.last_activity_ns IS NULL THEN sessions.last_activity_ns ELSE MAX(sessions.last_activity_ns,excluded.last_activity_ns) END,
  model=COALESCE(excluded.model,sessions.model),event_count=sessions.event_count+excluded.event_count,
  input_tokens=CASE WHEN excluded.input_tokens IS NULL THEN sessions.input_tokens WHEN sessions.input_tokens IS NULL THEN excluded.input_tokens ELSE sessions.input_tokens+excluded.input_tokens END,
  output_tokens=CASE WHEN excluded.output_tokens IS NULL THEN sessions.output_tokens WHEN sessions.output_tokens IS NULL THEN excluded.output_tokens ELSE sessions.output_tokens+excluded.output_tokens END,
  reasoning_tokens=CASE WHEN excluded.reasoning_tokens IS NULL THEN sessions.reasoning_tokens WHEN sessions.reasoning_tokens IS NULL THEN excluded.reasoning_tokens ELSE sessions.reasoning_tokens+excluded.reasoning_tokens END,
  data_quality=CASE WHEN excluded.data_quality='complete' THEN 'complete' ELSE sessions.data_quality END`,
		sessionID, sourceID, externalID, timestamp, timestamp, nullableString(model), eventDelta,
		inputTokens, outputTokens, reasoningTokens, quality, receivedAtNS)
}

func upsertTurn(ctx context.Context, tx *sql.Tx, turnID, sourceID, externalID, sessionID string, activityAt *int64, model string, inputTokens, outputTokens, reasoningTokens any, quality string) error {
	var timestamp any
	if activityAt != nil {
		timestamp = *activityAt
	}
	return execTx(ctx, tx, `INSERT INTO turns(turn_id,source_id,external_id,session_id,started_at_ns,last_activity_ns,model,input_tokens,output_tokens,reasoning_tokens,data_quality)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(turn_id) DO UPDATE SET
  session_id=COALESCE(excluded.session_id,turns.session_id),
  started_at_ns=CASE WHEN turns.started_at_ns IS NULL THEN excluded.started_at_ns WHEN excluded.started_at_ns IS NULL THEN turns.started_at_ns ELSE MIN(turns.started_at_ns,excluded.started_at_ns) END,
  last_activity_ns=CASE WHEN turns.last_activity_ns IS NULL THEN excluded.last_activity_ns WHEN excluded.last_activity_ns IS NULL THEN turns.last_activity_ns ELSE MAX(turns.last_activity_ns,excluded.last_activity_ns) END,
  model=COALESCE(excluded.model,turns.model),
  input_tokens=CASE WHEN excluded.input_tokens IS NULL THEN turns.input_tokens WHEN turns.input_tokens IS NULL THEN excluded.input_tokens ELSE turns.input_tokens+excluded.input_tokens END,
  output_tokens=CASE WHEN excluded.output_tokens IS NULL THEN turns.output_tokens WHEN turns.output_tokens IS NULL THEN excluded.output_tokens ELSE turns.output_tokens+excluded.output_tokens END,
  reasoning_tokens=CASE WHEN excluded.reasoning_tokens IS NULL THEN turns.reasoning_tokens WHEN turns.reasoning_tokens IS NULL THEN excluded.reasoning_tokens ELSE turns.reasoning_tokens+excluded.reasoning_tokens END,
  data_quality=CASE WHEN excluded.data_quality='complete' THEN 'complete' ELSE turns.data_quality END`,
		turnID, sourceID, externalID, nullableString(sessionID), timestamp, timestamp, nullableString(model),
		inputTokens, outputTokens, reasoningTokens, quality)
}

func persistToolCall(ctx context.Context, tx *sql.Tx, sourceID, sessionID, turnID, externalID, name, toolType string, startAt, endAt *int64, duration *float64, status string, retryIndex *int64, traceID, spanID string, eventID int64, correlationMethod string, correlationConfidence *float64) error {
	callID := makeToolCallID(sourceID, sessionID, externalID, traceID, spanID, eventID)
	if externalID != "" && traceID != "" && spanID != "" {
		var existingID string
		var existingSessionID, existingTurnID sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT tool_call_id,session_id,turn_id FROM tool_calls
		WHERE source_id=? AND external_tool_call_id=? AND trace_id=? AND span_id=?
		ORDER BY CASE WHEN session_id=? THEN 0 ELSE 1 END,tool_call_id LIMIT 1`,
			sourceID, externalID, traceID, spanID, nullableString(sessionID)).Scan(&existingID, &existingSessionID, &existingTurnID)
		if err == nil {
			sessionConflict := existingSessionID.Valid && sessionID != "" && existingSessionID.String != sessionID
			turnConflict := existingTurnID.Valid && turnID != "" && existingTurnID.String != turnID
			if !sessionConflict && !turnConflict {
				callID = existingID
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	method := "unassigned"
	var confidence any
	if sessionID != "" {
		method, confidence = "observed_identifier", 1.0
	} else if turnID != "" {
		method, confidence = "observed_turn_identifier", 1.0
	}
	if correlationMethod != "" && correlationMethod != "unassigned" {
		method = correlationMethod
	}
	if correlationConfidence != nil {
		confidence = *correlationConfidence
	}
	quality := "partial"
	if startAt != nil && (externalID != "" || traceID != "" && spanID != "") {
		quality = "complete"
	}
	return execTx(ctx, tx, `INSERT INTO tool_calls(tool_call_id,source_id,external_tool_call_id,session_id,turn_id,tool_name,tool_type,start_time_ns,end_time_ns,duration_ms,status,retry_index,trace_id,span_id,source_event_id,correlation_method,correlation_confidence,data_quality)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(tool_call_id) DO UPDATE SET
  session_id=CASE WHEN tool_calls.session_id IS NOT NULL AND excluded.session_id IS NOT NULL AND tool_calls.session_id!=excluded.session_id THEN tool_calls.session_id ELSE COALESCE(excluded.session_id,tool_calls.session_id) END,
  turn_id=CASE WHEN tool_calls.turn_id IS NOT NULL AND excluded.turn_id IS NOT NULL AND tool_calls.turn_id!=excluded.turn_id THEN tool_calls.turn_id ELSE COALESCE(excluded.turn_id,tool_calls.turn_id) END,
  tool_type=COALESCE(excluded.tool_type,tool_calls.tool_type),
  start_time_ns=CASE WHEN tool_calls.start_time_ns IS NULL THEN excluded.start_time_ns WHEN excluded.start_time_ns IS NULL THEN tool_calls.start_time_ns ELSE MIN(tool_calls.start_time_ns,excluded.start_time_ns) END,
  end_time_ns=COALESCE(excluded.end_time_ns,tool_calls.end_time_ns),duration_ms=COALESCE(excluded.duration_ms,tool_calls.duration_ms),
  status=COALESCE(excluded.status,tool_calls.status),retry_index=COALESCE(excluded.retry_index,tool_calls.retry_index),trace_id=COALESCE(excluded.trace_id,tool_calls.trace_id),span_id=COALESCE(excluded.span_id,tool_calls.span_id),
  source_event_id=COALESCE(excluded.source_event_id,tool_calls.source_event_id),
  correlation_method=CASE WHEN excluded.correlation_method!='unassigned' THEN excluded.correlation_method ELSE tool_calls.correlation_method END,
  correlation_confidence=COALESCE(excluded.correlation_confidence,tool_calls.correlation_confidence),
  data_quality=CASE WHEN excluded.data_quality='complete' THEN 'complete' ELSE tool_calls.data_quality END`,
		callID, sourceID, nullableString(externalID), nullableString(sessionID), nullableString(turnID), name,
		nullableString(toolType), nullableInt64Arg(startAt), nullableInt64Arg(endAt), nullableFloat64Arg(duration),
		nullableString(status), nullableInt64Arg(retryIndex), nullableString(traceID), nullableString(spanID), nullableEventID(eventID), method, confidence, quality)
}

func makeToolCallID(sourceID, sessionID, externalID, traceID, spanID string, eventID int64) string {
	if externalID != "" {
		prefix := sessionID
		if prefix == "" {
			prefix = sourceID
		}
		return prefix + ":tool:" + externalID
	}
	if traceID != "" && spanID != "" {
		return sourceID + ":span:" + traceID + ":" + spanID
	}
	return sourceID + ":event:" + strconv.FormatInt(eventID, 10)
}

func nullableInt64Arg(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat64Arg(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableEventID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func execTx(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func upsertObservedAttributeKeys(ctx context.Context, tx *sql.Tx, signal telemetry.Signal, keys []string, received int64) error {
	const maxObservedKeys = 256
	const maxKeyLength = 128
	key := "observed_attribute_keys_" + string(signal)
	observed := make(map[string]struct{})
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key=?`, key).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var prior []string
		if json.Unmarshal([]byte(existing), &prior) == nil {
			for _, name := range prior {
				observed[name] = struct{}{}
			}
		}
	}
	for _, name := range keys {
		if len(name) == 0 || len(name) > maxKeyLength || !validAttributeKey(name) {
			continue
		}
		if _, exists := observed[name]; exists || len(observed) < maxObservedKeys {
			observed[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(observed))
	for name := range observed {
		names = append(names, name)
	}
	sort.Strings(names)
	encoded, err := json.Marshal(names)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_state(key,value,updated_at_ns) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at_ns=excluded.updated_at_ns`, key, string(encoded), received)
	return err
}

func validAttributeKey(name string) bool {
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func safeName(value string) string {
	if len(value) > 128 {
		return value[:128]
	}
	return value
}
