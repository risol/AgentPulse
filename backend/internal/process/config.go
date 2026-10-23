package process

import (
	"fmt"
	"strings"
)

const (
	DefaultSampleIntervalMS = 500
	MinimumSampleIntervalMS = 250
	MaximumSampleIntervalMS = 2000
	DefaultRetentionDays    = 30
)

type Config struct {
	Enabled                bool           `json:"enabled"`
	SampleIntervalMS       int            `json:"sample_interval_ms"`
	RetentionDays          int            `json:"process_retention_days"`
	RootProcessNames       []string       `json:"root_process_names"`
	CategoryRules          []CategoryRule `json:"category_rules"`
	CaptureExecutablePaths bool           `json:"capture_executable_paths"`
	CaptureCommandLines    bool           `json:"capture_command_lines"`
}

type CategoryRule struct {
	Category     string   `json:"category"`
	ProcessNames []string `json:"process_names"`
}

func DefaultConfig() Config {
	return Config{
		SampleIntervalMS: DefaultSampleIntervalMS,
		RetentionDays:    DefaultRetentionDays,
		RootProcessNames: []string{"codex.exe"},
		CategoryRules:    DefaultCategoryRules(),
	}
}

func DefaultCategoryRules() []CategoryRule {
	return []CategoryRule{
		{Category: "shell", ProcessNames: []string{"cmd.exe", "powershell.exe", "pwsh.exe", "bash.exe", "sh.exe"}},
		{Category: "git", ProcessNames: []string{"git.exe"}},
		{Category: "build", ProcessNames: []string{"cmake.exe", "msbuild.exe", "ninja.exe", "make.exe", "cargo.exe", "rustc.exe", "flutter.exe", "dart.exe"}},
		{Category: "test", ProcessNames: []string{"pytest.exe", "testhost.exe", "vstest.console.exe", "flutter_tester.exe"}},
		{Category: "node", ProcessNames: []string{"node.exe", "npm.exe", "pnpm.exe", "yarn.exe"}},
		{Category: "python", ProcessNames: []string{"python.exe", "python3.exe"}},
		{Category: "install", ProcessNames: []string{"winget.exe", "choco.exe", "scoop.exe"}},
	}
}

func ValidateConfig(config Config) (Config, error) {
	if config.SampleIntervalMS < MinimumSampleIntervalMS || config.SampleIntervalMS > MaximumSampleIntervalMS {
		return Config{}, fmt.Errorf("sample_interval_ms must be between %d and %d", MinimumSampleIntervalMS, MaximumSampleIntervalMS)
	}
	// Older saved settings do not have this field. Give them the new default.
	if config.RetentionDays == 0 {
		config.RetentionDays = DefaultRetentionDays
	}
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return Config{}, fmt.Errorf("process_retention_days must be between 1 and 365")
	}
	if len(config.RootProcessNames) == 0 || len(config.RootProcessNames) > 8 {
		return Config{}, fmt.Errorf("root_process_names must contain between 1 and 8 names")
	}
	seen := make(map[string]struct{}, len(config.RootProcessNames))
	for index, name := range config.RootProcessNames {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" || len(name) > 64 || strings.ContainsAny(name, `/\\:`) {
			return Config{}, fmt.Errorf("root process name %d must be a filename without a path", index+1)
		}
		if !strings.HasSuffix(name, ".exe") {
			name += ".exe"
		}
		if name == ".exe" {
			return Config{}, fmt.Errorf("root process name %d is invalid", index+1)
		}
		if _, duplicate := seen[name]; duplicate {
			return Config{}, fmt.Errorf("root process name %q is duplicated", name)
		}
		seen[name] = struct{}{}
		config.RootProcessNames[index] = name
	}
	if config.CategoryRules == nil {
		config.CategoryRules = DefaultCategoryRules()
	}
	if len(config.CategoryRules) > 16 {
		return Config{}, fmt.Errorf("category_rules must contain at most 16 rules")
	}
	seenCategories := make(map[string]struct{}, len(config.CategoryRules))
	seenCategoryNames := make(map[string]struct{})
	totalCategoryNames := 0
	for ruleIndex := range config.CategoryRules {
		rule := &config.CategoryRules[ruleIndex]
		category := strings.TrimSpace(strings.ToLower(rule.Category))
		if !validCategoryName(category) {
			return Config{}, fmt.Errorf("category rule %d must use a short lowercase label", ruleIndex+1)
		}
		if _, duplicate := seenCategories[category]; duplicate {
			return Config{}, fmt.Errorf("category %q is duplicated", category)
		}
		seenCategories[category] = struct{}{}
		rule.Category = category
		if len(rule.ProcessNames) == 0 || len(rule.ProcessNames) > 64 {
			return Config{}, fmt.Errorf("category %q must contain between 1 and 64 process names", category)
		}
		for nameIndex, name := range rule.ProcessNames {
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" || len(name) > 64 || strings.ContainsAny(name, `/\\:`) {
				return Config{}, fmt.Errorf("category %q process name %d must be a filename without a path", category, nameIndex+1)
			}
			if !strings.HasSuffix(name, ".exe") {
				name += ".exe"
			}
			if name == ".exe" {
				return Config{}, fmt.Errorf("category %q contains an invalid process name", category)
			}
			if _, duplicate := seenCategoryNames[name]; duplicate {
				return Config{}, fmt.Errorf("process name %q appears in multiple category rules", name)
			}
			seenCategoryNames[name] = struct{}{}
			rule.ProcessNames[nameIndex] = name
			totalCategoryNames++
		}
	}
	if totalCategoryNames > 128 {
		return Config{}, fmt.Errorf("category_rules may contain at most 128 process names")
	}
	return config, nil
}

func cloneCategoryRules(rules []CategoryRule) []CategoryRule {
	if rules == nil {
		return nil
	}
	result := make([]CategoryRule, len(rules))
	for index, rule := range rules {
		result[index] = CategoryRule{
			Category:     rule.Category,
			ProcessNames: append([]string(nil), rule.ProcessNames...),
		}
	}
	return result
}

func validCategoryName(category string) bool {
	if len(category) == 0 || len(category) > 32 || category[0] < 'a' || category[0] > 'z' {
		return false
	}
	for _, char := range category[1:] {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}
