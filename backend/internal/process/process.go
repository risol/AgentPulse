// Package process models low-intrusion process snapshots and their lifecycle.
package process

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrUnsupported = errors.New("process monitoring is only available on Windows")

type Entry struct {
	PID       uint32
	ParentPID uint32
	ImageName string
}

type Sample struct {
	ProcessID                      string
	PID                            uint32
	ParentPID                      uint32
	ImageName                      string
	ExecutablePath                 string
	CommandLine                    string
	ExecutablePathCaptureAttempted bool
	CommandLineCaptureAttempted    bool
	StartedAtNS                    int64
	SampledAtNS                    int64
	CPUTimeNS                      *int64
	WorkingSetBytes                *uint64
}

type Record struct {
	ProcessID       string
	PID             uint32
	ParentPID       uint32
	ImageName       string
	ExecutablePath  string
	CommandLine     string
	StartedAtNS     int64
	OrderAtNS       int64
	EndedAtNS       *int64
	ExitCode        *uint32
	LastSampleAtNS  *int64
	CPUTimeMS       *float64
	WorkingSetBytes *uint64
	SampleQuality   string
}

type Snapshot struct {
	SampledAtNS     int64
	Processes       []Sample
	Exited          []ExitedProcess
	IncompleteCount int
}

type ExitedProcess struct {
	Process   Sample
	EndedAtNS int64
	ExitCode  *uint32
}

type ChangeSet struct {
	Started []Sample
	Updated []Sample
	Exited  []ExitedProcess
}

type Collector interface {
	Snapshot(ctx context.Context, config Config, previous []Sample) (Snapshot, error)
}

func SelectTree(entries []Entry, rootProcessNames []string, seedPIDs []uint32) []Entry {
	roots := make(map[string]struct{}, len(rootProcessNames))
	for _, name := range rootProcessNames {
		roots[normalizeName(name)] = struct{}{}
	}
	byPID := make(map[uint32]Entry, len(entries))
	for _, entry := range entries {
		byPID[entry.PID] = entry
	}
	selected := make(map[uint32]struct{})
	for _, entry := range entries {
		if _, match := roots[normalizeName(entry.ImageName)]; match {
			selected[entry.PID] = struct{}{}
		}
	}
	for _, pid := range seedPIDs {
		if _, exists := byPID[pid]; exists {
			selected[pid] = struct{}{}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, entry := range entries {
			if _, exists := selected[entry.PID]; exists {
				continue
			}
			if _, parentSelected := selected[entry.ParentPID]; parentSelected {
				selected[entry.PID] = struct{}{}
				changed = true
			}
		}
	}
	result := make([]Entry, 0, len(selected))
	for pid := range selected {
		result = append(result, byPID[pid])
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

func NormalizeProcessName(name string) string {
	return normalizeName(name)
}

// ClassifyImageName applies the user's exact executable-name rules. It does
// not imply that the process belongs to a particular tool call or session.
func ClassifyImageName(imageName string, rules []CategoryRule) (string, bool) {
	name := normalizeName(imageName)
	for _, rule := range rules {
		for _, processName := range rule.ProcessNames {
			if name == normalizeName(processName) {
				return rule.Category, true
			}
		}
	}
	return "", false
}

func normalizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimSuffix(name, ".exe")
}

type Tracker struct {
	active map[string]Sample
}

func NewTracker() *Tracker {
	return &Tracker{active: make(map[string]Sample)}
}

func (t *Tracker) Active() []Sample {
	result := make([]Sample, 0, len(t.active))
	for _, sample := range t.active {
		result = append(result, sample)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

func (t *Tracker) Apply(snapshot Snapshot) ChangeSet {
	if t.active == nil {
		t.active = make(map[string]Sample)
	}
	current := make(map[string]Sample, len(snapshot.Processes))
	changes := ChangeSet{
		Started: make([]Sample, 0), Updated: make([]Sample, 0, len(snapshot.Processes)),
		Exited: make([]ExitedProcess, 0),
	}
	for _, sample := range snapshot.Processes {
		if sample.ProcessID == "" {
			sample.ProcessID = InstanceID(sample.PID, sample.StartedAtNS)
		}
		if sample.SampledAtNS == 0 {
			sample.SampledAtNS = snapshot.SampledAtNS
		}
		current[sample.ProcessID] = sample
		if _, exists := t.active[sample.ProcessID]; !exists {
			changes.Started = append(changes.Started, sample)
		}
		changes.Updated = append(changes.Updated, sample)
	}
	for id, previous := range t.active {
		if _, exists := current[id]; !exists {
			exited := ExitedProcess{Process: previous, EndedAtNS: snapshot.SampledAtNS}
			for _, detail := range snapshot.Exited {
				if detail.Process.ProcessID == id {
					exited = detail
					if exited.EndedAtNS == 0 {
						exited.EndedAtNS = snapshot.SampledAtNS
					}
					break
				}
			}
			changes.Exited = append(changes.Exited, exited)
		}
	}
	t.active = current
	return changes
}

func InstanceID(pid uint32, startedAtNS int64) string {
	return fmt.Sprintf("%d:%d", pid, startedAtNS)
}
