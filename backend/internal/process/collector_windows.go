//go:build windows

package process

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
)

type windowsCollector struct{}

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func NewCollector() Collector { return windowsCollector{} }

func (windowsCollector) Snapshot(ctx context.Context, config Config, previous []Sample) (Snapshot, error) {
	entries, err := processEntries()
	if err != nil {
		return Snapshot{}, err
	}

	previousByPID := make(map[uint32]Sample, len(previous))
	for _, sample := range previous {
		previousByPID[sample.PID] = sample
	}
	seedPIDs := make([]uint32, 0, len(previous))
	validatedSamples := make(map[uint32]Sample, len(previous))
	entryByPID := make(map[uint32]Entry, len(entries))
	for _, entry := range entries {
		entryByPID[entry.PID] = entry
	}
	for pid, previousSample := range previousByPID {
		entry, exists := entryByPID[pid]
		if !exists {
			continue
		}
		capturePath := config.CaptureExecutablePaths && previousSample.ExecutablePath == "" && !previousSample.ExecutablePathCaptureAttempted
		current, err := readSample(entry, time.Now().UnixNano(), capturePath)
		if err == nil && current.ProcessID == previousSample.ProcessID {
			if !capturePath {
				current.ExecutablePath = previousSample.ExecutablePath
			}
			current.ExecutablePathCaptureAttempted = current.ExecutablePathCaptureAttempted || previousSample.ExecutablePathCaptureAttempted
			current.CommandLine = previousSample.CommandLine
			current.CommandLineCaptureAttempted = previousSample.CommandLineCaptureAttempted
			seedPIDs = append(seedPIDs, pid)
			validatedSamples[pid] = current
		}
	}
	selected := SelectTree(entries, config.RootProcessNames, seedPIDs)
	commandLines := make(map[uint32]string)
	if config.CaptureCommandLines {
		pids := make([]uint32, 0, len(selected))
		for _, entry := range selected {
			if sample, exists := validatedSamples[entry.PID]; !exists || !sample.CommandLineCaptureAttempted {
				pids = append(pids, entry.PID)
			}
		}
		commandLines, _ = readProcessCommandLines(pids)
	}

	result := Snapshot{SampledAtNS: time.Now().UnixNano(), Processes: make([]Sample, 0, len(selected))}
	for _, entry := range selected {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if sample, exists := validatedSamples[entry.PID]; exists {
			sample.SampledAtNS = result.SampledAtNS
			if config.CaptureCommandLines && !sample.CommandLineCaptureAttempted {
				sample.CommandLine = RedactCommandLine(commandLines[entry.PID])
				sample.CommandLineCaptureAttempted = true
			}
			if (config.CaptureExecutablePaths && sample.ExecutablePath == "") || (config.CaptureCommandLines && sample.CommandLine == "") {
				result.IncompleteCount++
			}
			result.Processes = append(result.Processes, sample)
			continue
		}
		sample, err := readSample(entry, result.SampledAtNS, config.CaptureExecutablePaths)
		if err != nil {
			result.IncompleteCount++
			continue
		}
		if config.CaptureCommandLines {
			sample.CommandLine = RedactCommandLine(commandLines[entry.PID])
			sample.CommandLineCaptureAttempted = true
		}
		if (config.CaptureExecutablePaths && sample.ExecutablePath == "") || (config.CaptureCommandLines && sample.CommandLine == "") {
			result.IncompleteCount++
		}
		result.Processes = append(result.Processes, sample)
	}
	currentIDs := make(map[string]struct{}, len(result.Processes))
	for _, sample := range result.Processes {
		currentIDs[sample.ProcessID] = struct{}{}
	}
	for _, sample := range previous {
		if _, stillActive := currentIDs[sample.ProcessID]; stillActive {
			continue
		}
		exited := ExitedProcess{Process: sample, EndedAtNS: result.SampledAtNS}
		if _, pidReused := entryByPID[sample.PID]; !pidReused {
			exited.ExitCode = readExitCode(sample.PID)
		}
		result.Exited = append(result.Exited, exited)
	}
	return result, nil
}

func processEntries() ([]Entry, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return []Entry{}, nil
		}
		return nil, err
	}
	entries := make([]Entry, 0, 256)
	for {
		entries = append(entries, Entry{
			PID: entry.ProcessID, ParentPID: entry.ParentProcessID,
			ImageName: strings.ToLower(windows.UTF16ToString(entry.ExeFile[:])),
		})
		entry.Size = uint32(unsafe.Sizeof(windows.ProcessEntry32{}))
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, err
		}
	}
	return entries, nil
}

func readSample(entry Entry, sampledAtNS int64, captureExecutablePath bool) (Sample, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.PID)
	if err != nil {
		return Sample{}, err
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return Sample{}, err
	}
	startedAt := creation.Nanoseconds()
	cpuNS := filetimeDurationNS(kernel) + filetimeDurationNS(user)
	sample := Sample{
		ProcessID: InstanceID(entry.PID, startedAt), PID: entry.PID, ParentPID: entry.ParentPID,
		ImageName: entry.ImageName, StartedAtNS: startedAt, SampledAtNS: sampledAtNS,
		CPUTimeNS: &cpuNS,
	}
	if captureExecutablePath {
		sample.ExecutablePathCaptureAttempted = true
		path, err := readExecutablePath(handle)
		if err == nil {
			sample.ExecutablePath = RedactExecutablePath(path)
		}
	}
	var memory processMemoryCounters
	memory.cb = uint32(unsafe.Sizeof(memory))
	ok, _, _ := getProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&memory)), uintptr(memory.cb))
	runtime.KeepAlive(&memory)
	if ok != 0 {
		workingSet := uint64(memory.workingSetSize)
		sample.WorkingSetBytes = &workingSet
	}
	return sample, nil
}

func readExecutablePath(handle windows.Handle) (string, error) {
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

type wmiProcessCommandLine struct {
	ProcessId   uint32
	CommandLine *string
}

func readProcessCommandLines(pids []uint32) (map[uint32]string, error) {
	result := make(map[uint32]string, len(pids))
	if len(pids) == 0 {
		return result, nil
	}
	const batchSize = 128
	for start := 0; start < len(pids); start += batchSize {
		end := start + batchSize
		if end > len(pids) {
			end = len(pids)
		}
		conditions := make([]string, 0, end-start)
		for _, pid := range pids[start:end] {
			conditions = append(conditions, "ProcessId = "+strconv.FormatUint(uint64(pid), 10))
		}
		query := "SELECT ProcessId, CommandLine FROM Win32_Process WHERE " + strings.Join(conditions, " OR ")
		var rows []wmiProcessCommandLine
		client := wmi.Client{PtrNil: true}
		if err := client.Query(query, &rows); err != nil {
			return result, err
		}
		for _, row := range rows {
			if row.CommandLine != nil {
				result[row.ProcessId] = *row.CommandLine
			}
		}
	}
	return result, nil
}

func filetimeDurationNS(value windows.Filetime) int64 {
	ticks := uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
	if ticks > uint64(^uint64(0)>>1)/100 {
		return 0
	}
	return int64(ticks * 100)
}

func readExitCode(pid uint32) *uint32 {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil || code == 259 {
		return nil
	}
	return &code
}
