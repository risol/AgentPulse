package process

import "testing"

func TestSelectTreeIncludesRootsDescendantsAndValidatedSeeds(t *testing.T) {
	entries := []Entry{
		{PID: 1, ImageName: "codex.exe"},
		{PID: 2, ParentPID: 1, ImageName: "node.exe"},
		{PID: 3, ParentPID: 2, ImageName: "rg.exe"},
		{PID: 4, ParentPID: 99, ImageName: "pwsh.exe"},
		{PID: 5, ParentPID: 4, ImageName: "git.exe"},
	}
	selected := SelectTree(entries, []string{"Codex"}, []uint32{4})
	if len(selected) != 5 {
		t.Fatalf("selected %d processes, want root tree and validated seed tree (5)", len(selected))
	}
	for index, want := range []uint32{1, 2, 3, 4, 5} {
		if selected[index].PID != want {
			t.Fatalf("selected PID at %d = %d, want %d", index, selected[index].PID, want)
		}
	}
}

func TestSelectTreeDoesNotIncludeUnrelatedProcesses(t *testing.T) {
	entries := []Entry{
		{PID: 1, ImageName: "codex.exe"},
		{PID: 2, ParentPID: 1, ImageName: "pwsh.exe"},
		{PID: 3, ParentPID: 99, ImageName: "git.exe"},
	}
	selected := SelectTree(entries, []string{"codex.exe"}, nil)
	if len(selected) != 2 || selected[0].PID != 1 || selected[1].PID != 2 {
		t.Fatalf("unexpected process tree: %+v", selected)
	}
}

func TestValidateConfigNormalizesAndRejectsUnsafeNames(t *testing.T) {
	config := DefaultConfig()
	if config.CaptureExecutablePaths || config.CaptureCommandLines {
		t.Fatal("sensitive process details must default to off")
	}
	config.RootProcessNames = []string{"Codex"}
	validated, err := ValidateConfig(config)
	if err != nil || validated.RootProcessNames[0] != "codex.exe" {
		t.Fatalf("valid root name was not normalized: %+v, error=%v", validated, err)
	}

	config.RootProcessNames = []string{`C:\tools\codex.exe`}
	if _, err := ValidateConfig(config); err == nil {
		t.Fatal("process paths should not be accepted as root names")
	}
	config = DefaultConfig()
	config.SampleIntervalMS = 100
	if _, err := ValidateConfig(config); err == nil {
		t.Fatal("out-of-range sample interval should be rejected")
	}
	config = DefaultConfig()
	config.RetentionDays = 366
	if _, err := ValidateConfig(config); err == nil {
		t.Fatal("retention beyond one year should be rejected")
	}
	config.RetentionDays = 0 // Missing in settings saved by earlier versions.
	validated, err = ValidateConfig(config)
	if err != nil || validated.RetentionDays != DefaultRetentionDays {
		t.Fatalf("legacy retention setting did not receive the default: %+v, error=%v", validated, err)
	}
}

func TestValidateConfigNormalizesCategoryRules(t *testing.T) {
	config := DefaultConfig()
	config.CategoryRules = []CategoryRule{{Category: " Build ", ProcessNames: []string{" MSBuild", "Ninja.exe"}}}
	validated, err := ValidateConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if validated.CategoryRules[0].Category != "build" || validated.CategoryRules[0].ProcessNames[0] != "msbuild.exe" || validated.CategoryRules[0].ProcessNames[1] != "ninja.exe" {
		t.Fatalf("category rules were not normalized: %+v", validated.CategoryRules)
	}

	config.CategoryRules = []CategoryRule{
		{Category: "build", ProcessNames: []string{"msbuild.exe"}},
		{Category: "test", ProcessNames: []string{"MSBuild"}},
	}
	if _, err := ValidateConfig(config); err == nil {
		t.Fatal("a process name assigned to multiple categories should be rejected")
	}
	config.CategoryRules = []CategoryRule{{Category: "Build tools", ProcessNames: []string{"msbuild.exe"}}}
	if _, err := ValidateConfig(config); err == nil {
		t.Fatal("a category label with unsupported characters should be rejected")
	}
}

func TestClassifyImageNameUsesExactConfiguredRules(t *testing.T) {
	rules := []CategoryRule{{Category: "shell", ProcessNames: []string{"pwsh.exe"}}}
	category, matched := ClassifyImageName("PWSH.EXE", rules)
	if !matched || category != "shell" {
		t.Fatalf("classification = (%q, %t), want (shell, true)", category, matched)
	}
	if category, matched = ClassifyImageName("pwsh-helper.exe", rules); matched || category != "" {
		t.Fatalf("prefix match should not classify an unrelated executable: (%q, %t)", category, matched)
	}
}

func TestTrackerDistinguishesReusedPID(t *testing.T) {
	tracker := NewTracker()
	first := Sample{PID: 10, StartedAtNS: 100, ProcessID: InstanceID(10, 100)}
	started := tracker.Apply(Snapshot{SampledAtNS: 110, Processes: []Sample{first}})
	if len(started.Started) != 1 {
		t.Fatalf("first snapshot started %d process(es), want 1", len(started.Started))
	}

	second := Sample{PID: 10, StartedAtNS: 200, ProcessID: InstanceID(10, 200)}
	changes := tracker.Apply(Snapshot{SampledAtNS: 210, Processes: []Sample{second}})
	if len(changes.Started) != 1 || len(changes.Exited) != 1 || changes.Exited[0].Process.ProcessID != first.ProcessID {
		t.Fatalf("PID reuse was not separated into exit/start: %+v", changes)
	}
}

func TestTrackerMarksProcessesExitedWhenTheyDisappear(t *testing.T) {
	tracker := NewTracker()
	sample := Sample{PID: 10, StartedAtNS: 100, ProcessID: InstanceID(10, 100)}
	tracker.Apply(Snapshot{SampledAtNS: 110, Processes: []Sample{sample}})
	changes := tracker.Apply(Snapshot{SampledAtNS: 200})
	if len(changes.Exited) != 1 || changes.Exited[0].EndedAtNS != 200 || len(tracker.Active()) != 0 {
		t.Fatalf("process exit was not recorded: %+v", changes)
	}
}

func TestTrackerKeepsExitCodeWhenAvailable(t *testing.T) {
	tracker := NewTracker()
	sample := Sample{PID: 10, StartedAtNS: 100, ProcessID: InstanceID(10, 100)}
	tracker.Apply(Snapshot{SampledAtNS: 110, Processes: []Sample{sample}})
	exitCode := uint32(7)
	changes := tracker.Apply(Snapshot{
		SampledAtNS: 200,
		Exited:      []ExitedProcess{{Process: sample, EndedAtNS: 190, ExitCode: &exitCode}},
	})
	if len(changes.Exited) != 1 || changes.Exited[0].ExitCode == nil || *changes.Exited[0].ExitCode != exitCode || changes.Exited[0].EndedAtNS != 190 {
		t.Fatalf("exit details were lost: %+v", changes.Exited)
	}
}
