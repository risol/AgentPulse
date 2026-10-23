# Changelog

## Unreleased — 2026-10-23

### Added

- Added schema v14 fields that preserve OTLP histogram, exponential histogram, and summary aggregation data.
- Added separate, default-off Process page settings for capturing executable paths and command lines.
- Added user-profile masking, common credential redaction, and a 4,096-rune limit for captured command lines.
- Added SQLite schema v13 migration for optional process path and command-line fields.
- Added session timeline de-duplication for synthetic trace events when the canonical span is present.
- Added privacy and security guidance and a reference for the persisted telemetry schema.
- Added contributor instructions, architecture decision records, and a Windows package/recovery runbook.

### Fixed

- Prefer a unique specific Codex command span over its generic `exec` envelope when associating a sampled shell process; keep multiple specific candidates unassigned.
- Configure an eight-connection SQLite pool so historical reads can overlap the bounded telemetry writer.

### Verified

- Rebuilt the Windows Release package with schema v14 and passed the isolated install, upgrade, uninstall, receiver-port, and forced-shutdown recovery smoke.
- Ran the isolated Windows UI E2E with 20 OTLP log records; dashboard and persisted counts matched, with telemetry-to-dashboard P95 at 315 ms. The Flutter test enabled codex.exe monitoring from the Process page at 500 ms, confirmed four persisted root records and the sampling state, then disabled monitoring without saving paths or command lines.
- Migrated the active local database to schema v14 with a verified pre-migration backup; confirmed `/health` returned `ready` and Codex resumed Logs, Metrics, and Traces without a Codex restart.
- Re-ran the synthetic 100,000-row performance check: standalone dashboard-refresh P95 was 431 ms and event-pagination P95 was 4 ms; while dashboard refresh ran concurrently, all 10/100/1,000 events/s ingest targets were accepted and refresh P95 was 565 ms.
- Verified the Windows Process page against live Codex descendants at 500 ms with path and command-line capture off; confirmed `exec_command` association and saved monitoring back to disabled.
- Passed the packaged fixed-workflow Windows E2E: 20 logs reached the dashboard and database (P95 356 ms), the UI/API/database agreed on a successful command, an expected failure, and a test program; command/test types and retry_index=1 on the expected failure were preserved and shown in the UI, and all three spawned PowerShell PIDs were explicitly associated. Monitoring was disabled at completion with path and command-line capture off.
- Re-ran the isolated Windows package install/upgrade/uninstall and recovery smoke on the rebuilt package; data-preservation/removal and custom receiver-port checks passed.