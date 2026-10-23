package codexconfig

import "github.com/agentpulse/agentpulse/backend/internal/application"

// Commands adapts the local configuration functions to an explicit
// application dependency.
type Commands struct {
	Endpoint string
}

func (commands Commands) Detect() (application.CodexConfigurationInfo, error) {
	info, err := DetectAtEndpoint(commands.endpoint())
	return toApplicationInfo(info), err
}

func (commands Commands) Preview() (application.CodexConfigurationPreview, error) {
	preview, _, err := BuildPreviewAtEndpoint(commands.endpoint())
	return application.CodexConfigurationPreview{
		Path: preview.Path, Exists: preview.Exists, Configured: preview.Configured,
		Changes: preview.Changes, Restart: preview.Restart,
	}, err
}

func (commands Commands) Apply() (application.CodexConfigurationInfo, string, error) {
	info, backup, err := ApplyAtEndpoint(commands.endpoint())
	return toApplicationInfo(info), backup, err
}

func (commands Commands) Restore(backupName string) (application.CodexConfigurationInfo, string, error) {
	info, recovery, err := RestoreAtEndpoint(commands.endpoint(), backupName)
	return toApplicationInfo(info), recovery, err
}

func (commands Commands) endpoint() string {
	if commands.Endpoint == "" {
		return defaultOTLPEndpoint()
	}
	return commands.Endpoint
}

func toApplicationInfo(info Info) application.CodexConfigurationInfo {
	return application.CodexConfigurationInfo{
		Path: info.Path, Exists: info.Exists, Configured: info.Configured, Backups: info.Backups,
	}
}

var _ application.CodexConfiguration = Commands{}
