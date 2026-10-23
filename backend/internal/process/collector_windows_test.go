//go:build windows

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const collectorChildProcessEnv = "AGENTPULSE_COLLECTOR_CHILD_PROCESS"

func TestWindowsCollectorSamplesCurrentTestProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("cannot identify test process")
	}
	pid := uint32(os.Getpid())
	config := DefaultConfig()
	config.RootProcessNames = []string{filepath.Base(executable)}
	config.CaptureExecutablePaths = true
	config.CaptureCommandLines = true
	snapshot, err := NewCollector().Snapshot(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("collect process snapshot: %v", err)
	}
	for _, sample := range snapshot.Processes {
		if sample.PID != pid {
			continue
		}
		if sample.ProcessID != InstanceID(pid, sample.StartedAtNS) || sample.StartedAtNS <= 0 {
			t.Fatalf("collector returned an invalid process identity: %+v", sample)
		}
		if sample.CPUTimeNS == nil || *sample.CPUTimeNS < 0 {
			t.Fatalf("collector did not return process CPU time: %+v", sample)
		}
		if sample.WorkingSetBytes == nil || *sample.WorkingSetBytes == 0 {
			t.Fatalf("collector did not return working set memory: %+v", sample)
		}
		if sample.ExecutablePath == "" || sample.CommandLine == "" {
			t.Fatalf("collector did not return opted-in process details: %+v", sample)
		}
		return
	}
	t.Fatal("snapshot did not include the current test process")
}

func TestWindowsCollectorOmitsSensitiveDetailsByDefault(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("cannot identify test process")
	}
	config := DefaultConfig()
	config.RootProcessNames = []string{filepath.Base(executable)}
	snapshot, err := NewCollector().Snapshot(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("collect process snapshot: %v", err)
	}
	for _, sample := range snapshot.Processes {
		if sample.PID == uint32(os.Getpid()) {
			if sample.ExecutablePath != "" || sample.CommandLine != "" {
				t.Fatalf("default collector captured sensitive details: %+v", sample)
			}
			return
		}
	}
	t.Fatal("snapshot did not include the current test process")
}

func TestWindowsCollectorSamplesLongRunningChildProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("cannot identify test process")
	}

	child := exec.Command(executable, "-test.run=^TestWindowsCollectorChildProcessHelper$")
	child.Env = append(os.Environ(), collectorChildProcessEnv+"=1")
	if err := child.Start(); err != nil {
		t.Fatalf("start long-running child process: %v", err)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})

	childPID := uint32(child.Process.Pid)
	config := DefaultConfig()
	config.RootProcessNames = []string{filepath.Base(executable)}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := NewCollector().Snapshot(context.Background(), config, nil)
		if err != nil {
			t.Fatalf("collect process snapshot: %v", err)
		}
		for _, sample := range snapshot.Processes {
			if sample.PID != childPID {
				continue
			}
			if sample.ParentPID != uint32(os.Getpid()) {
				t.Fatalf("child parent PID = %d, want %d", sample.ParentPID, os.Getpid())
			}
			if sample.ProcessID != InstanceID(childPID, sample.StartedAtNS) || sample.StartedAtNS <= 0 {
				t.Fatalf("collector returned an invalid child identity: %+v", sample)
			}
			if sample.CPUTimeNS == nil || sample.WorkingSetBytes == nil || *sample.WorkingSetBytes == 0 {
				t.Fatalf("collector omitted child resource data: %+v", sample)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("snapshot did not include long-running child process PID %d", childPID)
}

func TestWindowsCollectorChildProcessHelper(t *testing.T) {
	if os.Getenv(collectorChildProcessEnv) == "1" {
		time.Sleep(30 * time.Second)
	}
}
