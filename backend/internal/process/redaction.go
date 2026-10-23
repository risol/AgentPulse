package process

import (
	"os"
	"regexp"
	"strings"
)

const MaxStoredCommandLineRunes = 4096

var (
	commandFlagSecret = regexp.MustCompile(`(?i)(--?[a-z0-9_.-]*(?:token|secret|password|passwd|api[_-]?key|authorization|credential)[a-z0-9_.-]*)(\s*[:=]\s*|\s+)("[^"]*"|'[^']*'|[^\s]+)`)
	environmentSecret = regexp.MustCompile(`(?i)(\b[A-Z0-9_.-]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API[_-]?KEY|AUTHORIZATION|CREDENTIAL)[A-Z0-9_.-]*\s*=\s*)("[^"]*"|'[^']*'|[^\s]+)`)
)

// RedactExecutablePath replaces the current user's profile directory before
// an explicitly opted-in executable path is persisted.
func RedactExecutablePath(path string) string {
	return redactUserProfile(path)
}

// RedactCommandLine masks common credential arguments and the current user's
// profile directory. Callers should still explain that arbitrary argument
// values can contain sensitive content.
func RedactCommandLine(commandLine string) string {
	redacted := commandFlagSecret.ReplaceAllString(commandLine, "$1$2[redacted]")
	redacted = environmentSecret.ReplaceAllString(redacted, "$1[redacted]")
	redacted = redactUserProfile(redacted)
	runes := []rune(redacted)
	if len(runes) > MaxStoredCommandLineRunes {
		suffix := []rune("… [truncated]")
		return string(runes[:MaxStoredCommandLineRunes-len(suffix)]) + string(suffix)
	}
	return redacted
}

func redactUserProfile(value string) string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" || value == "" {
		return value
	}
	home = strings.TrimRight(home, `\\/`)
	lowerHome := strings.ToLower(home)
	lowerValue := strings.ToLower(value)
	index := strings.Index(lowerValue, lowerHome)
	if index < 0 {
		return value
	}
	var result strings.Builder
	for index >= 0 {
		result.WriteString(value[:index])
		result.WriteString("%USERPROFILE%")
		value = value[index+len(home):]
		lowerValue = lowerValue[index+len(home):]
		index = strings.Index(lowerValue, lowerHome)
	}
	result.WriteString(value)
	return result.String()
}
