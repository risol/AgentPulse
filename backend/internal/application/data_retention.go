package application

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrInvalidTelemetryRetention = errors.New("telemetry retention must be between 1 and 365 days")

const (
	TelemetryCleanupInterval  = time.Hour
	telemetryCleanupBatchSize = 500
)

type TelemetryRetentionRepository interface {
	LoadTelemetryRetentionDays(context.Context) (int, error)
	SaveTelemetryRetentionDays(context.Context, int) error
	PruneTelemetryHistoryBatch(context.Context, int64, int) (int64, error)
	DatabaseSizeBytes(context.Context) (int64, error)
	OptimizeDatabase(context.Context) error
}

type TelemetryRetentionStatus struct {
	Running         bool   `json:"running"`
	LastCleanupAtNS int64  `json:"last_cleanup_at_ns,omitempty"`
	LastDeletedRows int64  `json:"last_deleted_rows"`
	LastError       string `json:"last_error,omitempty"`
}

type TelemetryRetentionSnapshot struct {
	RetentionDays     int                      `json:"retention_days"`
	DatabaseSizeBytes int64                    `json:"database_size_bytes"`
	Status            TelemetryRetentionStatus `json:"status"`
}

type TelemetryRetentionService struct {
	repository TelemetryRetentionRepository
	mu         sync.RWMutex
	cleanupMu  sync.Mutex
	status     TelemetryRetentionStatus
	wake       chan struct{}
	done       chan struct{}
	startOnce  sync.Once
}

func NewTelemetryRetentionService(repository TelemetryRetentionRepository) *TelemetryRetentionService {
	return &TelemetryRetentionService{
		repository: repository,
		wake:       make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
}

func (s *TelemetryRetentionService) Snapshot(ctx context.Context) (TelemetryRetentionSnapshot, error) {
	days, err := s.repository.LoadTelemetryRetentionDays(ctx)
	if err != nil {
		return TelemetryRetentionSnapshot{}, err
	}
	size, err := s.repository.DatabaseSizeBytes(ctx)
	if err != nil {
		return TelemetryRetentionSnapshot{}, err
	}
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()
	return TelemetryRetentionSnapshot{RetentionDays: days, DatabaseSizeBytes: size, Status: status}, nil
}

func (s *TelemetryRetentionService) Update(ctx context.Context, days int) (TelemetryRetentionSnapshot, error) {
	if days < 1 || days > 365 {
		return TelemetryRetentionSnapshot{}, ErrInvalidTelemetryRetention
	}
	if err := s.repository.SaveTelemetryRetentionDays(ctx, days); err != nil {
		return TelemetryRetentionSnapshot{}, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return s.Snapshot(ctx)
}

// RequestCleanup queues a cleanup without tying an HTTP request to the duration
// of pruning a potentially large history.
func (s *TelemetryRetentionService) RequestCleanup() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *TelemetryRetentionService) CleanupNow(ctx context.Context) (int64, error) {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	s.setStatus(func(status *TelemetryRetentionStatus) {
		status.Running = true
		status.LastError = ""
	})
	completed := false
	defer func() {
		if !completed {
			s.setStatus(func(status *TelemetryRetentionStatus) { status.Running = false })
		}
	}()

	days, err := s.repository.LoadTelemetryRetentionDays(ctx)
	if err != nil {
		s.setStatus(func(status *TelemetryRetentionStatus) { status.LastError = "retention_settings_unavailable" })
		return 0, err
	}
	cutoffNS := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixNano()
	var total int64
	for {
		deleted, err := s.repository.PruneTelemetryHistoryBatch(ctx, cutoffNS, telemetryCleanupBatchSize)
		if err != nil {
			s.setStatus(func(status *TelemetryRetentionStatus) { status.LastError = "telemetry_cleanup_failed" })
			return total, err
		}
		total += deleted
		if deleted == 0 {
			break
		}
		select {
		case <-ctx.Done():
			s.setStatus(func(status *TelemetryRetentionStatus) { status.LastError = "telemetry_cleanup_interrupted" })
			return total, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := s.repository.OptimizeDatabase(ctx); err != nil {
		s.setStatus(func(status *TelemetryRetentionStatus) { status.LastError = "database_maintenance_failed" })
		return total, err
	}
	now := time.Now().UnixNano()
	s.setStatus(func(status *TelemetryRetentionStatus) {
		status.Running = false
		status.LastCleanupAtNS = now
		status.LastDeletedRows = total
		status.LastError = ""
	})
	completed = true
	return total, nil
}

func (s *TelemetryRetentionService) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		go func() {
			defer close(s.done)
			_, _ = s.CleanupNow(ctx)
			ticker := time.NewTicker(TelemetryCleanupInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-s.wake:
					_, _ = s.CleanupNow(ctx)
				case <-ticker.C:
					_, _ = s.CleanupNow(ctx)
				}
			}
		}()
	})
}

func (s *TelemetryRetentionService) Wait() {
	<-s.done
}

func (s *TelemetryRetentionService) setStatus(update func(*TelemetryRetentionStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.status)
}
