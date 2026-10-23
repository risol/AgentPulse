# Privacy and security

Last reviewed: 2026-10-23

AgentPulse is a local Windows application. Its receiver and query API bind to loopback, and the application does not upload telemetry or diagnostics.

## Collection defaults

| Data | Default behavior | User control |
| --- | --- | --- |
| Codex prompts | `otel.log_user_prompt` stays false; full OTLP request bodies are discarded. | The Codex settings preview avoids enabling prompt export. |
| OTLP attributes | Only reviewed attributes are persisted. Sensitive fields such as tool arguments/output, working directory, and account details are not value-allowlisted. | No arbitrary attribute capture setting is exposed. |
| Process history | Monitoring is disabled by default. When enabled, it samples configured root processes and visible descendants. | The Process page controls monitoring, roots, sampling interval, category rules, and 1–365 day retention. |
| Executable paths | Disabled by default and stored only for the selected process tree when enabled. | Independent **Save executable paths** switch in the Process page. The current user profile is replaced with `%USERPROFILE%`. |
| Command lines | Disabled by default and stored only for the selected process tree when enabled. | Independent **Save command-line arguments** switch in the Process page. Common credential flags/environment assignments and the current user profile are masked; values are capped at 4,096 runes. Other argument text may still contain prompts or secrets. |

Turning off path or command-line capture stops future collection. Existing values remain in process history until their retention period expires or the user clears history. The UI displays saved path and command-line values so users can review what was captured.

Process path and command-line data are separate from Codex OTLP attributes. Codex's observed attribute-name inventory records keys only and never stores their values.

## Local services and files

- OTLP receiver: loopback only, default port `4318`, bounded request size and admission queue. OTLP requests do not use bearer authentication; another local process able to reach the listener can submit telemetry.
- Application API: dynamic loopback port and random bearer token delivered to Flutter through the private child-process handshake. The token is not placed in process arguments or logs. The API rejects non-loopback Origins, checks the bearer token, and limits request rates.
- Database: `%APPDATA%\AgentPulse\agentpulse.db` on Windows, protected by the current user's filesystem permissions. Global telemetry retention defaults to 30 days and is configurable from 1–365 days. Process retention defaults to 30 days and applies to exited records and their samples.
- Application logs: local JSON logs with an attribute allowlist, 5 MiB rotation, at most three archives, and 14-day expiry.
- Diagnostics export: contains application/schema version, receiver state, queue counters, and sanitized diagnostic codes; it excludes event contents, process details, and Codex configuration.
- Database export: a consistent SQLite snapshot. It can include all retained telemetry and any process details the user enabled, so treat it as private local data.

The Settings page can clear telemetry and process history while preserving application settings. It can also remove expired application logs. Codex configuration changes are previewed and backed up before applying; the user can restore a selected backup.

## Limits

Credential masking covers common argument and environment-variable names. It cannot identify every secret or prompt embedded in arbitrary command arguments. Users should keep command-line capture off unless the added detail is needed and should treat an exported database as sensitive. Executable paths only mask the current user profile; other path segments may reveal local project or machine information.
