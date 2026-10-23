package application

import (
	"context"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

type IngestDiagnosticRepository interface {
	CountIngestDiagnosticsSince(context.Context, int64) (int64, error)
	ListRecentIngestDiagnostics(context.Context, int) ([]telemetry.IngestDiagnostic, error)
}

type IngestDiagnosticSnapshot struct {
	CountLast24Hours int64                        `json:"count_last_24_hours"`
	Recent           []telemetry.IngestDiagnostic `json:"recent"`
}

type IngestDiagnosticService struct {
	repository IngestDiagnosticRepository
}

func NewIngestDiagnosticService(repository IngestDiagnosticRepository) *IngestDiagnosticService {
	return &IngestDiagnosticService{repository: repository}
}

func (s *IngestDiagnosticService) Snapshot(ctx context.Context) (IngestDiagnosticSnapshot, error) {
	count, err := s.repository.CountIngestDiagnosticsSince(ctx, time.Now().Add(-24*time.Hour).UnixNano())
	if err != nil {
		return IngestDiagnosticSnapshot{}, err
	}
	recent, err := s.repository.ListRecentIngestDiagnostics(ctx, 10)
	if err != nil {
		return IngestDiagnosticSnapshot{}, err
	}
	if recent == nil {
		recent = []telemetry.IngestDiagnostic{}
	}
	return IngestDiagnosticSnapshot{CountLast24Hours: count, Recent: recent}, nil
}
