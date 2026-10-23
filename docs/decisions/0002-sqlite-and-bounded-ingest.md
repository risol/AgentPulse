# ADR 0002: Use SQLite with bounded telemetry ingestion

- Status: Accepted
- Date: 2026-10-23

## Context

The first product target is a single-user Windows desktop app. It needs local history, safe restart recovery, and bounded memory under export bursts without requiring an external database service.

## Decision

Use SQLite in WAL mode with versioned transactional migrations. OTLP requests decode into protocol-independent batches and enter a bounded single-worker write queue. A successful OTLP response is sent only after the corresponding batch commits. Queue capacity, payload size, request concurrency, and shutdown flush time are bounded.

## Consequences

- Installation remains self-contained and data remains local.
- Database schema changes have an explicit forward-only migration path.
- Writes are serialized; indexes, bounded batches, backpressure, and performance checks protect responsiveness.
- SQLite snapshots use `VACUUM INTO`; physical database compaction remains a separate disk-space concern.
