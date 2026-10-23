package process

import (
	"strings"
	"testing"
)

func TestRedactCommandLineMasksCredentialsAndUserProfile(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\agentpulse-test`)
	commandLine := `pwsh -File "C:\Users\agentpulse-test\project\run.ps1" --api-key=abc123 --password "secret value" OPENAI_TOKEN=another-secret`
	got := RedactCommandLine(commandLine)
	for _, secret := range []string{"abc123", "secret value", "another-secret", `C:\Users\agentpulse-test`} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted command line leaked %q: %q", secret, got)
		}
	}
	for _, expected := range []string{"--api-key=[redacted]", "--password [redacted]", "OPENAI_TOKEN=[redacted]", "%USERPROFILE%"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("redacted command line %q does not contain %q", got, expected)
		}
	}
}

func TestRedactCommandLineBoundsStoredLength(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	got := RedactCommandLine(strings.Repeat("x", MaxStoredCommandLineRunes+4))
	if len([]rune(got)) != MaxStoredCommandLineRunes {
		t.Fatalf("redacted command line rune count = %d", len([]rune(got)))
	}
}

func TestRedactExecutablePathUsesProfilePlaceholder(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\agentpulse-test`)
	got := RedactExecutablePath(`C:\Users\agentpulse-test\tools\pwsh.exe`)
	if strings.Contains(strings.ToLower(got), strings.ToLower(`C:\Users\agentpulse-test`)) || !strings.Contains(got, "%USERPROFILE%") {
		t.Fatalf("executable path was not redacted: %q", got)
	}
}
