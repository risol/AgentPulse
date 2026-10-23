package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

type telemetryOverviewRepositoryStub struct {
	lastReceived map[telemetry.Signal]int64
	eventCounts  map[telemetry.Signal]int64
	schema       int
	err          error
}

func (stub telemetryOverviewRepositoryStub) LastReceivedTimes(context.Context) (map[telemetry.Signal]int64, error) {
	return stub.lastReceived, stub.err
}

func (stub telemetryOverviewRepositoryStub) CountEventsSince(context.Context, int64) (map[telemetry.Signal]int64, error) {
	return stub.eventCounts, stub.err
}

func (stub telemetryOverviewRepositoryStub) SchemaVersion() int { return stub.schema }

func TestTelemetryOverviewClassifiesMissingStaleAndReceivingSignals(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	service := NewTelemetryOverviewService(telemetryOverviewRepositoryStub{
		schema: 12,
		lastReceived: map[telemetry.Signal]int64{
			telemetry.SignalLogs:    now.Add(-10 * time.Second).UnixNano(),
			telemetry.SignalMetrics: now.Add(-2 * time.Minute).UnixNano(),
		},
	})

	status, err := service.Status(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 12 || status.Signals[string(telemetry.SignalLogs)].State != "receiving" || status.Signals[string(telemetry.SignalMetrics)].State != "stale" || status.Signals[string(telemetry.SignalTraces)].State != "no_data" {
		t.Fatalf("unexpected signal status: %+v", status)
	}
	if status.Signals[string(telemetry.SignalLogs)].LastReceivedAt == "" || status.Signals[string(telemetry.SignalTraces)].LastReceivedAt != "" {
		t.Fatalf("last-received values do not match signal availability: %+v", status.Signals)
	}
}

func TestTelemetryOverviewSummaryValidatesRangeAndPreservesSignalCounts(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	service := NewTelemetryOverviewService(telemetryOverviewRepositoryStub{
		eventCounts: map[telemetry.Signal]int64{telemetry.SignalLogs: 4, telemetry.SignalTraces: 9},
	})

	summary, err := service.Summary(context.Background(), "15m", now)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Range != "15m" || summary.Events["logs"] != 4 || summary.Events["metrics"] != 0 || summary.Events["traces"] != 9 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if _, err := service.Summary(context.Background(), "forever", now); !errors.Is(err, ErrInvalidTelemetrySummaryRange) {
		t.Fatalf("invalid range error = %v", err)
	}
}
