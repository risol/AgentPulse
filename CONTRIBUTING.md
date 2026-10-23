# Contributing

`AgentPulse-Development-Plan.md` is the source of truth for product scope and phase acceptance. Read it together with [the architecture](docs/architecture.md), [the architecture review](docs/architecture-review.md), [the telemetry schema](docs/telemetry-schema.md), and [privacy and security](docs/privacy-and-security.md) before changing persistence, telemetry, process monitoring, or module boundaries.

## Development environment

- Windows 10 or newer for desktop and package work.
- Go 1.26 or newer.
- Flutter stable with the Windows desktop toolchain enabled.

Run the Go backend from `backend/`; run Flutter commands from `desktop/`. The development app starts the backend as a child process and uses a local SQLite database under `%APPDATA%\AgentPulse` unless a test or command supplies a temporary database path.

## Local verification

Run the checks relevant to the changed area before considering a change ready:

```powershell
Push-Location backend
$goFiles = rg --files . -g '*.go'
$unformatted = $goFiles | ForEach-Object { gofmt -l $_ } | Where-Object { $_ }
if ($unformatted) { $unformatted; throw 'Format Go source before continuing.' }
go test ./...
go build ./...
Pop-Location

Push-Location desktop
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test
Pop-Location

.\scripts\check-architecture.ps1
```

For a Windows release package, run:

```powershell
.\scripts\build-windows.ps1 -BuildDesktop
.\scripts\verify-windows-package.ps1 -PackagePath .\dist\AgentPulse-Windows-x64.zip
.\scripts\verify-windows-e2e.ps1 -PackagePath .\dist\AgentPulse-Windows-x64.zip
```

Both verifiers use isolated temporary profiles. The package verifier covers install, upgrade, uninstall, data retention/removal, custom receiver-port persistence, forced desktop termination, and backend recovery. The Windows E2E verifier sends local OTLP logs through the packaged backend, measures dashboard update latency, compares UI and persisted counts, and drives the Process page to briefly sample a selected process root before saving monitoring back to disabled. It verifies that path and command-line capture remain off. See [the Windows packaging runbook](docs/runbooks/windows-package.md).

The synthetic performance tool creates and removes its own temporary database; it does not use the user database:

```powershell
Push-Location backend
go run ./tools/perfcheck -events 100000 -samples 20 -rate-duration 2s
Pop-Location
```

## Change boundaries

- Keep `cmd/agentpulse` as the backend composition root. Transport handlers delegate domain and application work to the owning package.
- Put Codex-specific normalization in `internal/codexadapter`; keep source-specific attribute names out of general analytics and storage rules.
- Keep SQLite schema changes in ordered, transactional migrations. Preserve existing data and use NULL or explicit quality labels when evidence is missing.
- Keep prompt bodies and tool payloads out of persistence and logs. New sensitive process data needs an independent default-off setting, redaction, and privacy coverage in the UI and docs.
- Keep process monitoring opt-in. Do not change Codex permissions, sandbox settings, or execution behavior.
- Add sanitized protocol fixtures and focused tests for new parsing, mapping, correlation, or migration behavior.
- Keep Flutter feature modules independent; core and features must not import the composition root, and features must not import other features directly.

Hosted Windows CI runs for pushes and pull requests. Local check results do not imply that hosted CI ran.
