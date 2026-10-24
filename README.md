# AgentPulse

AgentPulse is a Windows-first, local-first observability app for AI coding agents. The first integration target is Codex Desktop. It is designed to explain task, model, tool, and process timing while making missing or estimated data visible.

AgentPulse now has a working local receiver, an authenticated loopback query API, and an early Flutter Windows dashboard. Codex Logs, Metrics, and Traces were observed from the installed client and redacted Protobuf fixtures were saved under `backend/tests/fixtures`. See [the integration capability matrix](docs/codex-integration.md).

## Build the backend

Requires Go 1.26 or newer.

```powershell
cd backend
go build -o agentpulse.exe ./cmd/agentpulse
./agentpulse.exe
```

The default database is `%APPDATA%\AgentPulse\agentpulse.db` on Windows. Use `-db` to choose another location or `-otlp-address` to choose another loopback port. Non-loopback addresses are rejected.

The receiver provides:

- `GET /health`
- `POST /v1/logs`
- `POST /v1/metrics`
- `POST /v1/traces`
- Authenticated status, dashboard summary, and Codex configuration endpoints on a separate dynamic loopback API. The Flutter app receives its random API token through a private startup handshake.

OTLP requests must use Protobuf content type and are limited to 4 MiB. Full request payloads are not stored. The initial allowlist retains only selected low-risk string, integer, and boolean attributes.

## Run the desktop app

Build both desktop components with:

```powershell
./scripts/build-windows.ps1 -BuildDesktop
```

This creates a runnable Windows distribution folder in `dist/`, including the Go launcher, Flutter app/runtime, and standalone backend. For development, build the backend into `backend/agentpulse.exe`, then run `flutter run -d windows` from `desktop/`.

The installed desktop shortcut runs `agentpulse-launcher.exe`: Go initializes logging, storage, the OTLP receiver, and the local API before starting the Flutter UI. It passes the API address and per-launch local token to Flutter through the child process environment. Flutter's direct startup path remains available for development hot reload and starts the backend itself when no launcher connection is supplied.

The dashboard shows signal counts for a selectable time range, live receiver status, and a separate paged list of events that could not be linked to a session. The Sessions page supports activity-time and model filters, cursor paging, observed activity spans, tool/command/failure counts, and session details with a zoomable waterfall and associated process tree. Process associations show whether a PID matched directly or a lower-confidence time/category rule was used. The source currently does not expose reliable session status or project paths. Logs and Traces are counted as events; Metrics are counted as metric points. Token totals use Logs to avoid double-counting usage repeated in traces and remain unavailable when no token value was observed. The Codex settings page detects the user config, previews the OTel changes, backs up and applies them, and restores a selected backup. It also lets users change the persistent loopback OTLP port (default `4318`); AgentPulse checks for a port conflict, restarts its receiver after a change, and refreshes the Codex endpoint preview. Applying Codex settings asks the user to restart Codex; connection is shown only when recent telemetry has arrived.

Persisted sessions, turns, tool calls, and event association metadata provide the canonical telemetry model. Session details include observed activity windows, partial interval/token/TTFT metrics, per-session tool summaries, and associated process records. Request-level linkage remains limited, and generation-stage durations remain unavailable without matching source spans. The tool explorer shows outcome counts, failure rate, P50/P95 durations, and a filterable, cursor-paged list of individual calls by session ID, time range, tool and result; retry indices appear only when Codex provides them. The Settings page configures global telemetry retention, cleans expired records and local application logs, exports a consistent SQLite snapshot and a sanitized diagnostics bundle, and can clear all telemetry and process history while preserving settings. The desktop refreshes through a coalescing SSE notification stream after telemetry is persisted. Overview charts show input/output tokens, exact-match request output rate, TTFT and average tool duration with missing-data gaps. `scripts/build-windows.ps1 -BuildDesktop` creates an offline ZIP bundle with per-user install, in-place upgrade and uninstall scripts; the latest candidate package passed the isolated lifecycle smoke run. Process monitoring is opt-in and provides configurable roots, sampling interval, executable-name category rules, cursor-paged process history, and retention for exited processes. The Process page also has separate, default-off controls for saving executable paths and command lines. Paths replace the current user profile with `%USERPROFILE%`; command lines mask common credential arguments and environment assignments, but other arguments may still contain sensitive text. Process-to-tool associations use exact span PIDs where available and conservative, confidence-labelled time/category rules otherwise; ambiguous cases remain unassigned. Current live Codex spans did not include `process.pid`; a Windows process-monitor run verified the conservative time/category association to `exec_command` at 0.55 confidence. Direct PID association and broader task-type/retry-index coverage remain unverified. Pure interval and token-rate calculations are implemented in the backend.

See [AgentPulse-Development-Plan.md](AgentPulse-Development-Plan.md) for the complete product plan.
See [the implementation status](docs/implementation-status.md) for completed and outstanding phases, and [the architecture review](docs/architecture-review.md) for the alignment assessment.
See [privacy and security](docs/privacy-and-security.md) for local data handling and capture controls, and [the telemetry schema](docs/telemetry-schema.md) for SQLite entities and migration policy.
See [contributing](CONTRIBUTING.md) for local checks, and [the Windows package runbook](docs/runbooks/windows-package.md) for build, install, recovery, and data-preservation steps.

Run `cd backend; go run ./tools/perfcheck -events 100000 -samples 20 -rate-duration 2s` to measure ingest rates and dashboard/page latency on a temporary synthetic database. It removes that database on exit and does not read the user's collection data.
