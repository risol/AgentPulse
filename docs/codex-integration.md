# Codex integration capability matrix

**Status: three signals verified; Gate 0 evidence captured.** The active user config on this development machine is `%USERPROFILE%\.codex\config.toml`; the installed CLI reports version `0.130.0-alpha.5`. The installed Codex Desktop package manifest reports `26.1016.8622.0` (`ChatGPT.exe` product version `155.0.8059.80`), and the telemetry resource reports `app.version = 0.163.0-alpha.11.2`. Real Codex telemetry arrived over the configured local OTLP/HTTP binary endpoints. The config was backed up before enabling exporters. Receiver restart recovery and endpoint conflict behavior were observed during development; redacted Protobuf fixtures are stored in `backend/tests/fixtures/`.

## Configuration documented by OpenAI

The current OpenAI Codex configuration reference lists `~/.codex/config.toml` as the user-level file. It documents OTLP HTTP and gRPC exporters, binary and JSON protocols, exporter endpoints and headers, separate metrics and trace exporter controls, and `otel.log_user_prompt` as an opt-in setting. Project-level `.codex/config.toml` cannot override telemetry routing keys. The current user config had no OTEL section before this setup. The exact tagged-exporter TOML form is also shown in the [Codex OTEL crate examples](https://github.com/openai/codex/blob/main/codex-rs/otel/README.md).

The applied config uses the documented signal-specific OTLP/HTTP shape:

```toml
[otel]
log_user_prompt = false
exporter = { "otlp-http" = { endpoint = "http://127.0.0.1:4318/v1/logs", protocol = "binary" } }
metrics_exporter = { "otlp-http" = { endpoint = "http://127.0.0.1:4318/v1/metrics", protocol = "binary" } }
trace_exporter = { "otlp-http" = { endpoint = "http://127.0.0.1:4318/v1/traces", protocol = "binary" } }
```

The original config backup is `%USERPROFILE%\.codex\config.toml.backup.20261023-115006`. The Codex Desktop process started at 2026-10-23 11:52:39 local time; the first stored Logs and Traces arrived at 11:55:07 and Metrics at 11:55:49. This confirms that exports resumed after Codex startup with a receiver listening. After authorization to replace the previous receiver, the AgentPulse candidate took over `127.0.0.1:4318`; the database migrated from schema 12 to 13. Codex remained open during replacement and all three signals continued to arrive. A read-only sample through 2026-10-23 10:45:36 UTC, since the candidate backend started at 10:24:41 UTC, contained 566 Logs, 1,027 Metrics, and 147,989 Traces. No span in that window included a process PID, and the latest 100 tool calls had no retry index. The database's last write before recovery was at 18:46:36 local time; at 18:51 no AgentPulse process or listener was present, and the logs did not record why. After I relaunched the candidate at 18:52:06, `/health` returned `ready` with schema 13 and all three signals arrived again by 18:52:53. On 2026-10-23 at 19:04, AgentPulse was gracefully stopped for the v14 upgrade and a full SQLite backup was saved to `%APPDATA%\AgentPulse\backups\agentpulse.pre-v14-20261023-190616.db`. The rebuilt v14 backend took over 4318, migrated the active database, and returned `ready` with schema 14. Comparing the stopped snapshot with the running database at 19:11:08 local time shows 167 new Logs, 291 new Metrics, and 47,060 new Traces. The latest receipt time for each signal advanced after restart. The running receiver has persisted 291 live histogram aggregation records with valid JSON, including count, sum, bounds, and bucket counts. On 2026-10-23, an authorized Windows Flutter integration run enabled monitoring from the Process page for codex.exe at 500 ms, with executable-path and command-line capture off. The short validation runs persisted 193 process records and 11,633 samples; a read-only database check found zero paths and zero command lines, and the UI saved monitoring back to disabled. The captured powershell.exe child (PID 39788, parent Codex PID 24444) overlapped both an outer exec span and a specific exec_command span, while neither span exported process PID fields. The correlation rule now prefers a unique specific command span over its generic exec envelope and leaves multiple specific candidates unassigned. The updated Windows integration test verified the time-and-category association to exec_command at 0.55 confidence. After the final backend build and restart, `/health` returned `ready` with schema 14; event/span receipt times advanced to 19:57:08 local time and metric receipt time to 19:56:53, confirming Codex resumed without a Codex restart. AgentPulse does not restart Codex.

## Capability matrix

| Capability | Current status | Evidence / next action |
| --- | --- | --- |
| User-level config path | Confirmed for the active CLI user config | `%USERPROFILE%\.codex\config.toml` exists; `CODEX_HOME` is unset. Installed Desktop package manifest: `26.1016.8622.0`; `ChatGPT.exe` product version: `155.0.8059.80`. |
| OTLP/HTTP exporter setting | Configured | Log, metric, and trace exporters point to local signal-specific endpoints. |
| Protobuf (`binary`) protocol | Verified | Receiver accepts OTLP Protobuf, and real Codex requests for all three signals were decoded and persisted. Redacted Protobuf fixtures are retained for regression coverage. |
| Logs export | Verified | Real `POST /v1/logs` requests were decoded and persisted. Observed keys include `conversation.id`, `model`, `input_token_count`, `output_token_count`, `ttft_ms`, `tool_name`, and `success`. |
| Metrics export | Verified | Real `POST /v1/metrics` requests were decoded; examples include `codex.tool.call`, `codex.tool.call.duration_ms`, `codex.websocket.event`, and `codex.websocket.event.duration_ms`. After the v14 restart, live Codex data populated 291 histogram aggregation records with valid count, sum, bounds, and bucket counts; recent examples include `codex.sqlite.logs.write.duration_ms` and `codex.rollout_compression.read.io_duration_ms`. |
| Traces export | Verified | Real `POST /v1/traces` spans were decoded and persisted. Observed keys include `session.id`, `conversation.id`, `turn.id`, `tool.name`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, and `codex.usage.reasoning_output_tokens`. |
| Export headers / authentication | Active configuration confirmed | No OTLP headers are configured; the receiver does not require them and only binds to loopback. Codex's configurable header support was not separately exercised. |
| Session / turn IDs, model, token counts, tool calls | Present in observed payloads | Allowlisted values are persisted; identifier values in fixtures are replaced. Coverage across task types still needs sampling. |
| Process PID attributes | Accepted when explicitly present on a span | `process.pid` and `process.parent_pid` are allowlisted for exact or parent-based association. No spans since the current receiver start included these fields, so command-span PID export remains unverified. |
| Prompt export | Disabled by application guidance | Keep `otel.log_user_prompt` unset/false; raw bodies are never stored by the receiver. |
| Restart and disconnect recovery | Verified for Codex startup, receiver replacement, and app relaunch | After Codex Desktop started at 11:52:39 local time, all three signals arrived by 11:55:49. During the receiver replacement, Codex remained open and all three signals continued. After the candidate app was relaunched at 18:52:06, it became healthy and all three signals resumed by 18:52:53. |

## Receiver support

| Signal | Endpoint | Decoder / persistence |
| --- | --- | --- |
| Logs | `POST /v1/logs` | OTLP Protobuf logs; stores timestamps, trace/span IDs, receipt time, and allowlisted attributes. Log body is discarded. |
| Metrics | `POST /v1/metrics` | OTLP Protobuf gauge, sum, histogram, exponential histogram, and summary data points. Scalar values and distribution fields—including counts, sums, bounds, buckets, quantiles, scale, temporality, and monotonicity—are persisted; datapoint attributes are allowlisted. |
| Traces | `POST /v1/traces` | OTLP Protobuf spans; stores trace/span relationships and timing. Span name is replaced with `span`; span events and links are not persisted yet. |

## Observed export cadence

This receiver assigns one `received_at_ns` timestamp to each accepted OTLP request. Grouping records by that timestamp provides an observed batch cadence for the active Codex run. The snapshot below covers the receiver start at 2026-10-23 10:24:41 UTC through 10:45:36 UTC; it is a measurement, not a configured exporter interval.

| Signal | Batches | Mean records per batch | Mean interval | Observed interval range |
| --- | ---: | ---: | ---: | ---: |
| Logs | 194 | 2.92 | 6.40 s | 0.151–33.293 s |
| Metrics | 31 | 33.13 | 40.01 s | 3.043–60.018 s |
| Traces | 461 | 321.02 spans | 2.73 s | 0.007–10.242 s |

The updated sample through 2026-10-23 10:45:36 UTC included 566 log records, 1,027 metric event records, and 147,989 trace events. Metrics included sums, histograms, and gauges; for example, `codex.tool.call` is a sum and `codex.tool.call.duration_ms` is a histogram. Export timing and batch size vary, so consumers should use event timestamps and must not assume fixed-size batches or exact periodic delivery.

## Measures and limits

Real payloads confirm that model labels, conversation/turn/session IDs, input/output/reasoning/cached token counts, TTFT, tool names, call IDs, outcomes, and duration fields can occur in this Codex build. Schema v14 persists source-aware session, turn, tool-call and normalized token records, optional process path/command-line fields, and full numeric OTLP histogram, exponential histogram, and summary aggregations. Events inherit session/turn identifiers from an exact matching Trace/Span across Logs and Traces, regardless of arrival order; conflicting IDs are preserved, while events with no usable ID or matching span remain unassigned and browsable through the local API and overview UI. Session analysis exposes observed activity windows, source event IDs, quality-labelled interval/token metrics, and per-session tool summaries. Trend charts use canonical token fields; token totals use Logs when present and remain unavailable otherwise, avoiding duplicate aggregation across signals. Request linkage, session end semantics, process PID attributes, and coverage across task types still need follow-up validation; generation-stage and process-derived command durations remain unavailable unless the source provides sufficient evidence.

The observed key inventory also includes sensitive fields such as `arguments`, `output`, `cwd`, `user.email`, and `user.account_id`. The receiver stores only attribute names in its observed-key inventory; values for those sensitive fields are not allowlisted. Raw request bodies are not logged or stored. Do not configure Codex to export raw prompts for this receiver. Process executable paths and command lines are a separate local collection feature, disabled by default and controlled in the Process page; see [privacy and security](privacy-and-security.md) for the redaction limits.

## Target-machine validation checklist

- [x] Record active CLI version (`0.130.0-alpha.5`), installed Desktop package version (`26.1016.8622.0`), user config path, and `CODEX_HOME` state.
- [x] Configure local OTLP/HTTP binary endpoints without enabling raw prompt export; back up the original file first.
- [x] Run a real Codex task in the active development session; logs, metrics, and traces arrived.
- [x] Confirm all three signals and record observed key names and metric names without retaining raw payloads.
- [x] Review allowlisted fields: token counts, model, TTFT, tool names, conversation/turn/session identifiers are present in observed payloads.
- [x] Restart the receiver while Codex remains open and confirm exports resume.
- [x] Replace the schema v13 receiver with the schema v14 build, migrate a full database, and confirm all three signals resumed without restarting Codex.
- [x] Verify port-conflict behavior; a second receiver fails clearly while the original listener remains healthy.
- [x] Store only redacted fixtures; exact wall-clock times and identifier values are sanitized.
- [x] Confirm Codex startup with the receiver continuously running; all three signals arrived within about three minutes of Desktop startup.
- [x] Run a 15-second shell command through the active Codex tool and inspect its matching span. At 19:25 local time on 2026-10-23, the persisted `exec_command` span lasted about 10.2 seconds and both `process.pid` and `process.parent_pid` were absent.
- [x] Temporarily enabled the opt-in monitor for `codex.exe` through the Windows Process page with path and command-line capture off. A real Codex shell command produced a sampled PowerShell child; the process tree and span overlap were inspected, and the specific `exec_command` span was associated by time and shell category at 0.55 confidence. Monitoring was saved as disabled afterward. The package E2E also exercises this UI path and verifies four persisted Codex roots without paths or command lines.
