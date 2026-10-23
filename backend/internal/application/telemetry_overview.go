package application

import (
	"context"
	"errors"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

var ErrInvalidTelemetrySummaryRange = errors.New("invalid telemetry summary range")

type TelemetryOverviewRepository interface {
	LastReceivedTimes(context.Context) (map[telemetry.Signal]int64, error)
	CountEventsSince(context.Context, int64) (map[telemetry.Signal]int64, error)
	SchemaVersion() int
}

type SignalHealth struct {
	State          string `json:"state"`
	LastReceivedAt string `json:"last_received_at,omitempty"`
}

type TelemetryOverviewStatus struct {
	SchemaVersion int                     `json:"schema_version"`
	Signals       map[string]SignalHealth `json:"signals"`
}

type TelemetryOverviewSummary struct {
	Range  string           `json:"range"`
	Events map[string]int64 `json:"events"`
}

// TelemetryOverviewService owns signal-health classification and the
// range-bounded dashboard event summary shared by the status and overview APIs.
type TelemetryOverviewService struct {
	repository TelemetryOverviewRepository
}

func NewTelemetryOverviewService(repository TelemetryOverviewRepository) *TelemetryOverviewService {
	return &TelemetryOverviewService{repository: repository}
}

func (s *TelemetryOverviewService) Status(ctx context.Context, now time.Time) (TelemetryOverviewStatus, error) {
	lastReceived, err := s.repository.LastReceivedTimes(ctx)
	if err != nil {
		return TelemetryOverviewStatus{}, err
	}
	signals := make(map[string]SignalHealth, 3)
	for _, signal := range []telemetry.Signal{telemetry.SignalLogs, telemetry.SignalMetrics, telemetry.SignalTraces} {
		receivedAt, exists := lastReceived[signal]
		if !exists {
			signals[string(signal)] = SignalHealth{State: "no_data"}
			continue
		}
		state := "stale"
		age := now.Sub(time.Unix(0, receivedAt))
		if age >= 0 && age <= time.Minute {
			state = "receiving"
		}
		signals[string(signal)] = SignalHealth{
			State: state, LastReceivedAt: time.Unix(0, receivedAt).UTC().Format(time.RFC3339),
		}
	}
	return TelemetryOverviewStatus{SchemaVersion: s.repository.SchemaVersion(), Signals: signals}, nil
}

func (s *TelemetryOverviewService) Summary(ctx context.Context, rangeName string, now time.Time) (TelemetryOverviewSummary, error) {
	rangeDuration, okay := SummaryRangeDuration(rangeName)
	if !okay {
		return TelemetryOverviewSummary{}, ErrInvalidTelemetrySummaryRange
	}
	events, err := s.repository.CountEventsSince(ctx, now.Add(-rangeDuration).UnixNano())
	if err != nil {
		return TelemetryOverviewSummary{}, err
	}
	return TelemetryOverviewSummary{
		Range: rangeName,
		Events: map[string]int64{
			string(telemetry.SignalLogs):    events[telemetry.SignalLogs],
			string(telemetry.SignalMetrics): events[telemetry.SignalMetrics],
			string(telemetry.SignalTraces):  events[telemetry.SignalTraces],
		},
	}, nil
}
