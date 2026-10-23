# Redacted Codex OTLP fixtures

These Protobuf payloads were reconstructed from the AgentPulse database after a real Codex task emitted all three OTLP signals. They contain allowlisted attributes only. Identifier values are replaced with fixture labels or per-run salted one-way hashes, and timestamps are shifted to an artificial epoch. Span names and log bodies are generic/omitted. Do not replace these files with raw exporter payloads.

Regenerate from a local database with:

```powershell
cd backend
go run ./tools/fixturegen -db "$env:APPDATA\AgentPulse\agentpulse.db" -out tests/fixtures
```

Files: `codex-logs.pb`, `codex-metrics.pb`, and `codex-traces.pb`.
