package application

// CodexConfigurationInfo is the application-facing state of the user config.
type CodexConfigurationInfo struct {
	Path       string   `json:"path"`
	Exists     bool     `json:"exists"`
	Configured bool     `json:"configured"`
	Backups    []string `json:"backups"`
}

// CodexConfigurationPreview contains only user-reviewable changes. The adapter
// keeps its optimistic-concurrency checksum private to the apply workflow.
type CodexConfigurationPreview struct {
	Path       string            `json:"path"`
	Exists     bool              `json:"exists"`
	Configured bool              `json:"configured"`
	Changes    map[string]string `json:"changes"`
	Restart    bool              `json:"restart_required"`
}

// CodexConfiguration is the application port for the local Codex file workflow.
// The platform adapter owns TOML parsing and filesystem changes.
type CodexConfiguration interface {
	Detect() (CodexConfigurationInfo, error)
	Preview() (CodexConfigurationPreview, error)
	Apply() (CodexConfigurationInfo, string, error)
	Restore(string) (CodexConfigurationInfo, string, error)
}
