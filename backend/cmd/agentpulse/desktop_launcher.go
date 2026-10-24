package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	apiAddressEnvironment = "AGENTPULSE_API_ADDRESS"
	apiTokenEnvironment   = "AGENTPULSE_API_TOKEN"
	backendVersionEnv     = "AGENTPULSE_BACKEND_VERSION"
)

func resolveDesktopExecutable(override string) (string, error) {
	if override != "" {
		path, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", fmt.Errorf("desktop executable is unavailable: %s", path)
		}
		return path, nil
	}

	launcher, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "agentpulse"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(launcher), name)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", fmt.Errorf("desktop executable is unavailable: %s", path)
	}
	return path, nil
}

func startDesktopProcess(executable, apiAddress, apiToken, backendVersion string) (*exec.Cmd, error) {
	command := exec.Command(executable)
	command.Dir = filepath.Dir(executable)
	command.Env = os.Environ()
	command.Env = setEnvironment(command.Env, apiAddressEnvironment, apiAddress)
	command.Env = setEnvironment(command.Env, apiTokenEnvironment, apiToken)
	command.Env = setEnvironment(command.Env, backendVersionEnv, backendVersion)
	command.Stdout = desktopLogWriter{level: slog.LevelInfo, stream: "stdout"}
	command.Stderr = desktopLogWriter{level: slog.LevelWarn, stream: "stderr"}
	if err := command.Start(); err != nil {
		return nil, err
	}
	slog.Info("desktop UI started", "executable", executable, "pid", command.Process.Pid)
	return command, nil
}

func setEnvironment(environment []string, key, value string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, hasValue := strings.Cut(entry, "=")
		if !hasValue {
			filtered = append(filtered, entry)
			continue
		}
		matches := name == key
		if runtime.GOOS == "windows" {
			matches = strings.EqualFold(name, key)
		}
		if !matches {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, key+"="+value)
}

type desktopLogWriter struct {
	level  slog.Level
	stream string
}

func (writer desktopLogWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message != "" {
		slog.Default().Log(context.Background(), writer.level, "desktop UI output", "stream", writer.stream, "message", message)
	}
	return len(data), nil
}

var _ io.Writer = desktopLogWriter{}
