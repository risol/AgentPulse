# Telemetry schema

Last reviewed: 2026-10-23

AgentPulse stores local telemetry in SQLite. The current database schema is **v14**. The schema is an internal persistence contract; API responses and UI models are mapped separately and may evolve independently.

## Storage rules

- SQLite runs in WAL mode. Schema changes are applied by ordered, transactional migrations in `backend/internal/storage/sqlite.go`.
- A database older than the application is migrated forward. The application refuses a database with a newer schema version.
- OTLP requests are decoded into normalized batches. Full request bodies are not persisted.
- Runtime attributes pass through an allowlist. The observed-key inventory stores attribute names only, not values.
- Missing identifiers, timestamps, and measurements remain NULL or unassigned; the application does not invent zero values or force an association.

## Main entities

| Table | Purpose |
| --- | --- |
| `telemetry_events` | Normalized Logs, Metrics, and Traces event records, with source IDs, timestamps, trace/span IDs, allowlisted attributes, token fields, and session/turn association. |
| `spans` | Trace and span timing, parent IDs, role/model, process IDs when supplied by the source, and session/turn association. `(trace_id, span_id)` is the primary key. |
| `sessions`, `turns` | Source-aware canonical records built from observed external identifiers, with activity windows, model and token summaries, and data-quality labels. |
| `tool_calls` | Normalized tool name, outcome, timing, retry index when supplied, source event, span IDs, and correlation quality. Input/output summaries are not populated from raw payloads. |
| `metric_points` | Numeric metric values with start/end time, temporality and monotonicity, allowlisted attributes, and numeric histogram/summary aggregation fields in `aggregation_json`. |
| `processes`, `process_samples` | Opt-in Windows process history and sampled resource usage. Process identity includes PID and start time to distinguish PID reuse. |
| `app_state`, `ingest_diagnostics` | Application settings, receiver state, bounded diagnostic codes, and observed OTLP attribute key names. |

Session and turn identifiers are source-aware. Events with an exact matching Trace/Span pair inherit the span's session/turn metadata across Logs and Traces regardless of arrival order. Conflicting identifiers are preserved. Events without a usable match remain unassigned and are available through the local API.

## Schema history

- v1–v3: initial event, span, metric, application-state, and process storage.
- v4: canonical session, turn, and tool-call entities plus observed association metadata and legacy backfill.
- v5–v10: tool time-range index, receipt-time tracking, bounded ingest diagnostics, span roles/models, process IDs and TTFT, then canonical token counts with a one-time allowlisted backfill.
- v11: exact cross-signal Trace/Span correlation index.
- v12: covering indexes for signal receipt-time counts and signal/Trace/Span lookups.
- v13: nullable `processes.executable_path` and `processes.command_line` fields. Existing rows remain NULL; values are captured only when their independent process settings are enabled.
- v14: `metric_points` records start time, aggregation temporality, sum monotonicity, and the numeric distribution data required to reconstruct Histogram, ExponentialHistogram, and Summary points. Existing metrics retain their scalar values and receive an empty aggregation object.

Histogram aggregation JSON retains count, sum, minimum/maximum when provided, explicit bounds and bucket counts, exponential scale/zero bucket/positive and negative buckets, summary quantiles, and data-point flags. Exemplars are dropped because the application does not use exemplar trace data.

Every future persisted format change must receive a new migration. Historical backfills must remain one-time and source-aware.
