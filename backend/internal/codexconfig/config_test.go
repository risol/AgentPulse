package codexconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestPreviewApplyAndRestorePreserveUnrelatedCodexSettings(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CODEX_HOME", directory)
	path := filepath.Join(directory, "config.toml")
	original := []byte("# user settings\nmodel = \"gpt-test\"\n[notice]\nshow_legacy_warning = false\n[otel]\ncustom = \"keep-me\"\nlog_user_prompt = true\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	preview, updated, err := BuildPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Restart || !preview.Configured || len(preview.Changes) != 4 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(original) {
		t.Fatal("preview modified the user's config")
	}
	var document map[string]any
	if err := toml.Unmarshal(updated, &document); err != nil {
		t.Fatalf("preview produced invalid TOML: %v", err)
	}
	if document["model"] != "gpt-test" || document["notice"].(map[string]any)["show_legacy_warning"] != false || document["otel"].(map[string]any)["custom"] != "keep-me" || document["otel"].(map[string]any)["log_user_prompt"] != false {
		t.Fatalf("preview failed to preserve settings or disable prompt logging: %#v", document)
	}

	info, backupName, err := Apply()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Configured || backupName == "" {
		t.Fatalf("apply returned config=%+v backup=%q", info, backupName)
	}
	backup, err := os.ReadFile(filepath.Join(directory, backupName))
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("apply backup does not contain the original user config")
	}

	_, recoveryBackup, err := Restore(backupName)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryBackup == "" {
		t.Fatal("restore did not preserve the config it replaced")
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Fatal("restore did not return the original config")
	}
}

func TestPreviewRejectsInvalidTOMLWithoutChangingFile(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CODEX_HOME", directory)
	path := filepath.Join(directory, "config.toml")
	invalid := []byte("[otel\ninvalid")
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildPreview(); err == nil || !strings.Contains(err.Error(), "有效 TOML") {
		t.Fatalf("invalid TOML error = %v", err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(invalid) {
		t.Fatal("invalid TOML was modified")
	}
}

func TestPreviewUsesConfiguredLoopbackReceiverPort(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CODEX_HOME", directory)
	endpoint := "http://127.0.0.1:4545"
	preview, updated, err := BuildPreviewAtEndpoint(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Configured {
		t.Fatalf("preview did not recognize the configured receiver endpoint: %+v", preview)
	}
	var document map[string]any
	if err := toml.Unmarshal(updated, &document); err != nil {
		t.Fatalf("preview produced invalid TOML: %v", err)
	}
	otel := document["otel"].(map[string]any)
	for key, signalPath := range map[string]string{
		"exporter":         "/v1/logs",
		"metrics_exporter": "/v1/metrics",
		"trace_exporter":   "/v1/traces",
	} {
		exporter := otel[key].(map[string]any)["otlp-http"].(map[string]any)
		if exporter["endpoint"] != endpoint+signalPath {
			t.Fatalf("%s endpoint = %v, want %s%s", key, exporter["endpoint"], endpoint, signalPath)
		}
	}
}

func TestConfigurationRejectsNonLoopbackReceiverEndpoint(t *testing.T) {
	for _, endpoint := range []string{"https://127.0.0.1:4318", "http://example.com:4318", "http://127.0.0.1:0"} {
		if _, _, err := BuildPreviewAtEndpoint(endpoint); err == nil {
			t.Errorf("accepted unsafe or invalid receiver endpoint %q", endpoint)
		}
	}
}

func TestRestoreRejectsBackupPathTraversal(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CODEX_HOME", directory)
	if _, _, err := Restore("..\\config.toml.backup.fake"); err == nil {
		t.Fatal("restore accepted a path outside the Codex config directory")
	}
}
