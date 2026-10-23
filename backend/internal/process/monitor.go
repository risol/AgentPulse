package process

import (
	"context"
	"sync"
	"time"
)

type MonitorStore interface {
	ApplyProcessSnapshot(context.Context, Snapshot, ChangeSet) error
	CloseActiveProcesses(context.Context, int64) error
	PruneProcessHistory(context.Context, int64) (int64, error)
}

const ProcessCleanupInterval = time.Hour

type MonitorStatus struct {
	Enabled         bool   `json:"enabled"`
	State           string `json:"state"`
	LastSampledAtNS int64  `json:"last_sampled_at_ns,omitempty"`
	ActiveProcesses int    `json:"active_processes"`
	IncompleteCount int    `json:"incomplete_count"`
	LastError       string `json:"last_error,omitempty"`
	LastCleanupAtNS int64  `json:"last_cleanup_at_ns,omitempty"`
	LastCleanupRows int64  `json:"last_cleanup_rows"`
	CleanupError    string `json:"cleanup_error,omitempty"`
}

type Monitor struct {
	store     MonitorStore
	collector Collector
	mu        sync.RWMutex
	config    Config
	status    MonitorStatus
	wake      chan struct{}
	done      chan struct{}
	startOnce sync.Once
}

func NewMonitor(store MonitorStore, collector Collector, config Config) (*Monitor, error) {
	validated, err := ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	validated.RootProcessNames = append([]string(nil), validated.RootProcessNames...)
	validated.CategoryRules = cloneCategoryRules(validated.CategoryRules)
	return &Monitor{
		store: store, collector: collector, config: validated,
		status: MonitorStatus{Enabled: validated.Enabled, State: "disabled"},
		wake:   make(chan struct{}, 1), done: make(chan struct{}),
	}, nil
}

func (m *Monitor) Configure(config Config) error {
	validated, err := ValidateConfig(config)
	if err != nil {
		return err
	}
	validated.RootProcessNames = append([]string(nil), validated.RootProcessNames...)
	validated.CategoryRules = cloneCategoryRules(validated.CategoryRules)
	m.mu.Lock()
	m.config = validated
	m.status.Enabled = validated.Enabled
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *Monitor) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := m.config
	result.RootProcessNames = append([]string(nil), m.config.RootProcessNames...)
	result.CategoryRules = cloneCategoryRules(m.config.CategoryRules)
	return result
}

func (m *Monitor) Status() MonitorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Monitor) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		go func() {
			defer close(m.done)
			m.run(ctx)
		}()
	})
}

func (m *Monitor) Wait() {
	<-m.done
}

func (m *Monitor) run(ctx context.Context) {
	tracker := NewTracker()
	_ = m.store.CloseActiveProcesses(ctx, time.Now().UnixNano())
	wasEnabled := false
	lastCleanupAttempt := time.Time{}
	lastCleanupRetentionDays := 0
	for {
		config := m.Config()
		now := time.Now()
		if config.RetentionDays != lastCleanupRetentionDays || now.Sub(lastCleanupAttempt) >= ProcessCleanupInterval {
			lastCleanupAttempt = now
			lastCleanupRetentionDays = config.RetentionDays
			cutoff := now.Add(-time.Duration(config.RetentionDays) * 24 * time.Hour).UnixNano()
			deleted, err := m.store.PruneProcessHistory(ctx, cutoff)
			if err != nil {
				m.setStatus(func(status *MonitorStatus) {
					status.CleanupError = "process_retention_cleanup_failed"
				})
			} else {
				m.setStatus(func(status *MonitorStatus) {
					status.LastCleanupAtNS = now.UnixNano()
					status.LastCleanupRows = deleted
					status.CleanupError = ""
				})
			}
		}
		if !config.Enabled {
			if wasEnabled {
				_ = m.store.CloseActiveProcesses(ctx, time.Now().UnixNano())
				tracker = NewTracker()
				wasEnabled = false
			}
			m.setStatus(func(status *MonitorStatus) {
				status.Enabled = false
				status.State = "disabled"
				status.ActiveProcesses = 0
				status.IncompleteCount = 0
				status.LastError = ""
			})
			timer := time.NewTimer(ProcessCleanupInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-m.wake:
				timer.Stop()
				continue
			case <-timer.C:
				continue
			}
		}
		wasEnabled = true
		m.setStatus(func(status *MonitorStatus) {
			status.Enabled = true
			status.State = "sampling"
		})
		snapshot, err := m.collector.Snapshot(ctx, config, tracker.Active())
		if err != nil {
			m.setStatus(func(status *MonitorStatus) {
				status.State = "error"
				status.LastError = "process_snapshot_failed"
			})
		} else {
			changes := tracker.Apply(snapshot)
			if err := m.store.ApplyProcessSnapshot(ctx, snapshot, changes); err != nil {
				m.setStatus(func(status *MonitorStatus) {
					status.State = "error"
					status.LastError = "process_storage_failed"
				})
			} else {
				m.setStatus(func(status *MonitorStatus) {
					status.State = "sampling"
					status.LastSampledAtNS = snapshot.SampledAtNS
					status.ActiveProcesses = len(snapshot.Processes)
					status.IncompleteCount = snapshot.IncompleteCount
					status.LastError = ""
				})
			}
		}
		timer := time.NewTimer(time.Duration(config.SampleIntervalMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if wasEnabled {
				_ = m.store.CloseActiveProcesses(context.Background(), time.Now().UnixNano())
			}
			return
		case <-m.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (m *Monitor) setStatus(update func(*MonitorStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&m.status)
}
