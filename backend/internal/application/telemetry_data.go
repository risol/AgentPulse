package application

import (
	"context"
	"io"
)

type TelemetryDataRepository interface {
	CreateDatabaseSnapshot(context.Context) (io.ReadCloser, int64, error)
	ClearHistory(context.Context) (int64, error)
}

// TelemetryDataService exposes user-requested export and clear-history
// operations without letting HTTP transport call the storage adapter directly.
type TelemetryDataService struct {
	repository TelemetryDataRepository
}

func NewTelemetryDataService(repository TelemetryDataRepository) *TelemetryDataService {
	return &TelemetryDataService{repository: repository}
}

func (s *TelemetryDataService) ExportSnapshot(ctx context.Context) (io.ReadCloser, int64, error) {
	return s.repository.CreateDatabaseSnapshot(ctx)
}

func (s *TelemetryDataService) ClearHistory(ctx context.Context) (int64, error) {
	return s.repository.ClearHistory(ctx)
}
