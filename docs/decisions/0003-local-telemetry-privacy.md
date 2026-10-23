# ADR 0003: Keep telemetry local and collection allowlisted

- Status: Accepted
- Date: 2026-10-23

## Context

Codex telemetry and process metadata can contain prompts, repository paths, command arguments, and credentials. The product is intended to work locally and should collect only information needed for the current analysis.

## Decision

Bind the OTLP receiver and authenticated application API to loopback. Persist allowlisted telemetry attributes and discard full request bodies. Keep prompt export disabled. Process monitoring is opt-in; executable paths and command lines have separate default-off controls. Apply profile masking and common credential redaction when either process detail is enabled.

## Consequences

- User configuration and the UI expose collection status and capture choices directly.
- Command-line redaction is best-effort; arbitrary argument values can still contain sensitive text.
- The loopback OTLP receiver has no bearer authentication, so another local process can submit telemetry.
- Database snapshots can contain retained telemetry and opt-in process details and must be treated as private data.
