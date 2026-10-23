package codexconfig

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/application"
	"github.com/pelletier/go-toml/v2/unstable/edit"
)

type Info struct {
	Path       string   `json:"path"`
	Exists     bool     `json:"exists"`
	Configured bool     `json:"configured"`
	Backups    []string `json:"backups"`
}

type Preview struct {
	Path         string            `json:"path"`
	Exists       bool              `json:"exists"`
	Configured   bool              `json:"configured"`
	Changes      map[string]string `json:"changes"`
	Restart      bool              `json:"restart_required"`
	originalHash string
}

func ConfigPath() (string, error) {
	var path string
	if home := os.Getenv("CODEX_HOME"); home != "" {
		path = filepath.Join(home, "config.toml")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, ".codex", "config.toml")
	}
	return filepath.Abs(path)
}

func Detect() (Info, error) {
	return DetectAtEndpoint(defaultOTLPEndpoint())
}

func DetectAtEndpoint(endpoint string) (Info, error) {
	if err := validateOTLPEndpoint(endpoint); err != nil {
		return Info{}, err
	}
	path, err := ConfigPath()
	if err != nil {
		return Info{}, err
	}
	info := Info{Path: path}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Info{}, err
	}
	if err == nil {
		info.Exists = true
		configured, err := isConfigured(data, endpoint)
		if err != nil {
			return Info{}, errors.New("Codex 配置无法安全读取或解析")
		}
		info.Configured = configured
	}
	matches, _ := filepath.Glob(path + ".backup.*")
	for _, match := range matches {
		info.Backups = append(info.Backups, filepath.Base(match))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(info.Backups)))
	return info, nil
}

func BuildPreview() (Preview, []byte, error) {
	return BuildPreviewAtEndpoint(defaultOTLPEndpoint())
}

func BuildPreviewAtEndpoint(endpoint string) (Preview, []byte, error) {
	if err := validateOTLPEndpoint(endpoint); err != nil {
		return Preview{}, nil, err
	}
	path, err := ConfigPath()
	if err != nil {
		return Preview{}, nil, err
	}
	original, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Preview{}, nil, err
	}
	doc, err := edit.Parse(original)
	if err != nil {
		return Preview{}, nil, errors.New("Codex 配置不是有效 TOML；未做任何修改")
	}
	if err := applySettings(doc, endpoint); err != nil {
		return Preview{}, nil, err
	}
	updated := doc.Bytes()
	configured, err := isConfigured(updated, endpoint)
	if err != nil {
		return Preview{}, nil, err
	}
	checksum := sha256.Sum256(original)
	return Preview{
		Path: path, Exists: exists, Configured: configured,
		originalHash: fmt.Sprintf("%x", checksum),
		Changes: map[string]string{
			"otel.exporter":         "OTLP/HTTP → http://127.0.0.1:4318/v1/logs (binary)",
			"otel.metrics_exporter": "OTLP/HTTP → http://127.0.0.1:4318/v1/metrics (binary)",
			"otel.trace_exporter":   "OTLP/HTTP → http://127.0.0.1:4318/v1/traces (binary)",
			"otel.log_user_prompt":  "false",
		},
		Restart: !bytes.Equal(original, updated),
	}, updated, nil
}

func Apply() (Info, string, error) {
	return ApplyAtEndpoint(defaultOTLPEndpoint())
}

func ApplyAtEndpoint(endpoint string) (Info, string, error) {
	preview, updated, err := BuildPreviewAtEndpoint(endpoint)
	if err != nil {
		return Info{}, "", err
	}
	if !preview.Restart {
		info, err := DetectAtEndpoint(endpoint)
		return info, "", err
	}
	backup, err := backupOriginal(preview.Path)
	if err != nil {
		return Info{}, "", err
	}
	backupData, err := os.ReadFile(backup)
	if err != nil {
		return Info{}, "", err
	}
	checksum := sha256.Sum256(backupData)
	if fmt.Sprintf("%x", checksum) != preview.originalHash {
		return Info{}, "", errors.New("Codex 配置在预览后发生变化；请重新预览")
	}
	if err := atomicWrite(preview.Path, updated, 0o600); err != nil {
		return Info{}, "", fmt.Errorf("write Codex config (backup kept at %s): %w", backup, err)
	}
	info, err := DetectAtEndpoint(endpoint)
	return info, filepath.Base(backup), err
}

func Restore(backupName string) (Info, string, error) {
	return RestoreAtEndpoint(defaultOTLPEndpoint(), backupName)
}

func RestoreAtEndpoint(endpoint, backupName string) (Info, string, error) {
	if err := validateOTLPEndpoint(endpoint); err != nil {
		return Info{}, "", err
	}
	path, err := ConfigPath()
	if err != nil {
		return Info{}, "", err
	}
	if filepath.Base(backupName) != backupName || !strings.HasPrefix(backupName, "config.toml.backup.") {
		return Info{}, "", errors.New("invalid backup name")
	}
	backupPath := filepath.Join(filepath.Dir(path), backupName)
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return Info{}, "", err
	}
	if _, err := edit.Parse(data); err != nil {
		return Info{}, "", errors.New("备份不是有效 TOML；未恢复文件")
	}
	recovery, err := backupOriginal(path)
	if err != nil {
		return Info{}, "", err
	}
	if err := atomicWrite(path, data, 0o600); err != nil {
		return Info{}, "", fmt.Errorf("restore failed (current config saved to %s): %w", recovery, err)
	}
	info, err := DetectAtEndpoint(endpoint)
	return info, filepath.Base(recovery), err
}

func applySettings(doc *edit.Document, endpoint string) error {
	values := []struct {
		key   string
		value any
	}{
		{"exporter", map[string]any{"otlp-http": map[string]any{"endpoint": endpoint + "/v1/logs", "protocol": "binary"}}},
		{"metrics_exporter", map[string]any{"otlp-http": map[string]any{"endpoint": endpoint + "/v1/metrics", "protocol": "binary"}}},
		{"trace_exporter", map[string]any{"otlp-http": map[string]any{"endpoint": endpoint + "/v1/traces", "protocol": "binary"}}},
		{"log_user_prompt", false},
	}
	for _, setting := range values {
		if err := doc.Set([]string{"otel", setting.key}, setting.value); err != nil {
			return fmt.Errorf("cannot set otel.%s safely", setting.key)
		}
	}
	return nil
}

func isConfigured(data []byte, endpoint string) (bool, error) {
	doc, err := edit.Parse(data)
	if err != nil {
		return false, err
	}
	for key, signalPath := range map[string]string{"exporter": "/v1/logs", "metrics_exporter": "/v1/metrics", "trace_exporter": "/v1/traces"} {
		value, ok := doc.Get([]string{"otel", key})
		if !ok {
			return false, nil
		}
		outer, ok := value.(map[string]any)
		if !ok {
			return false, nil
		}
		exporter, ok := outer["otlp-http"].(map[string]any)
		if !ok || exporter["endpoint"] != endpoint+signalPath || exporter["protocol"] != "binary" {
			return false, nil
		}
	}
	prompt, ok := doc.Get([]string{"otel", "log_user_prompt"})
	return ok && prompt == false, nil
}

func defaultOTLPEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", application.DefaultOTLPPort)
}

func validateOTLPEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("OTLP endpoint must be a local HTTP origin")
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return errors.New("OTLP endpoint must include a loopback host and port")
	}
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return errors.New("OTLP endpoint must use a loopback address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("OTLP endpoint port is invalid")
	}
	return nil
}

func backupOriginal(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	original, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err != nil {
		original = nil
	}
	backup := path + ".backup." + time.Now().Format("20060102-150405.000000000")
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err = file.Write(original); err != nil {
		file.Close()
		return "", err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	verified, err := os.ReadFile(backup)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(original, verified) {
		return "", fmt.Errorf("backup verification failed for %s", backup)
	}
	if sha256.Sum256(original) != sha256.Sum256(verified) {
		return "", fmt.Errorf("backup checksum verification failed for %s", backup)
	}
	return backup, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agentpulse-config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := edit.Parse(data); err != nil {
		return fmt.Errorf("refuse to write invalid TOML: %w", err)
	}
	return os.Rename(tmpPath, path)
}
