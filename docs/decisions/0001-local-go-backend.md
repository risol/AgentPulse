# ADR 0001: Run the Go backend as a local child process

- Status: Accepted
- Date: 2026-10-23

## Context

Telemetry ingestion, SQLite writes, process sampling, and retention work must continue independently from Flutter frame rendering. The backend also needs a repeatable integration boundary for tests and future local clients.

## Decision

Flutter starts and owns a separate Go backend process. The backend returns a versioned startup handshake over stdout containing its dynamic loopback API address, random bearer token, receiver address, and version. The token is not passed in command-line arguments or written to logs. Closing the parent pipe initiates graceful shutdown; bounded timeouts provide a kill fallback.

## Consequences

- The UI and backend can fail and recover independently.
- Local API requests are authenticated, and the API port does not need a fixed value.
- Startup, readiness, and shutdown are observable through an explicit contract.
- Packaging must ship both executables and verify their lifecycle together.
