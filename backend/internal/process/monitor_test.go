package process

import (
	"context"
	"testing"
	"time"
)

func TestMonitorCanBeEnabledAndDisabledAtRuntime(t *testing.T) {
	cpu := int64(1)
	memory := uint64(1024)
	sample := Sample{ProcessID: InstanceID(1, 1), PID: 1, StartedAtNS: 1, CPUTimeNS: &cpu, WorkingSetBytes: &memory}
	collector := &testCollector{snapshot: Snapshot{SampledAtNS: 10, Processes: []Sample{sample}}}
	store := &testMonitorStore{
		applied: make(chan Snapshot, 4), closures: make(chan int64, 4),
		pruned: make(chan int64, 4),
	}
	config := DefaultConfig()
	config.Enabled = true
	config.SampleIntervalMS = 2000
	monitor, err := NewMonitor(store, collector, config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)
	defer func() {
		cancel()
		monitor.Wait()
	}()
	select {
	case cutoff := <-store.pruned:
		wantCutoff := time.Now().Add(-time.Duration(config.RetentionDays) * 24 * time.Hour).UnixNano()
		if delta := cutoff - wantCutoff; delta < -int64(time.Minute) || delta > int64(time.Minute) {
			t.Fatalf("retention cutoff = %d, want close to %d", cutoff, wantCutoff)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not run process history retention cleanup")
	}

	select {
	case <-store.applied:
	case <-time.After(2 * time.Second):
		t.Fatal("enabled monitor did not persist a process snapshot")
	}
	if err := monitor.Configure(Config{Enabled: false, SampleIntervalMS: 500, RootProcessNames: []string{"codex.exe"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if monitor.Status().State == "disabled" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("monitor did not enter disabled state: %+v", monitor.Status())
}

func TestMonitorPrunesHistoryWhileCollectionIsDisabled(t *testing.T) {
	store := &testMonitorStore{
		applied: make(chan Snapshot, 1), closures: make(chan int64, 1),
		pruned: make(chan int64, 1),
	}
	monitor, err := NewMonitor(store, &testCollector{}, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)
	select {
	case <-store.pruned:
		cancel()
		monitor.Wait()
	case <-time.After(2 * time.Second):
		cancel()
		monitor.Wait()
		t.Fatal("disabled process monitor did not perform retention cleanup")
	}
}

type testCollector struct {
	snapshot Snapshot
}

func (c *testCollector) Snapshot(context.Context, Config, []Sample) (Snapshot, error) {
	return c.snapshot, nil
}

type testMonitorStore struct {
	applied  chan Snapshot
	closures chan int64
	pruned   chan int64
}

func (s *testMonitorStore) ApplyProcessSnapshot(_ context.Context, snapshot Snapshot, _ ChangeSet) error {
	select {
	case s.applied <- snapshot:
	default:
	}
	return nil
}

func (s *testMonitorStore) CloseActiveProcesses(_ context.Context, endedAtNS int64) error {
	select {
	case s.closures <- endedAtNS:
	default:
	}
	return nil
}

func (s *testMonitorStore) PruneProcessHistory(_ context.Context, cutoffNS int64) (int64, error) {
	select {
	case s.pruned <- cutoffNS:
	default:
	}
	return 0, nil
}
