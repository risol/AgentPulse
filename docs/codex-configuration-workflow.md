# Codex configuration workflow

The Settings UI must use the same guarded workflow as manual setup:

1. **Detect** the active user config (`%CODEX_HOME%\config.toml` when `CODEX_HOME` is set; otherwise `%USERPROFILE%\.codex\config.toml`). Show the path, whether the file exists, and the detected Codex version when available.
2. **Preview** the exact log, metrics, and trace endpoint changes. Keep `otel.log_user_prompt` false. Do not enable agent response or guardian assessment logging. Never read, display, or modify unrelated settings.
3. **Back up** the original file to a timestamped sibling file and verify that the backup matches the original bytes.
4. **Apply** only the OTel keys, using a TOML-aware merge that preserves unrelated values, comments, and formatting. Validate the resulting TOML before replacing the original atomically. If the active file cannot be safely parsed or has conflicting exporter settings, stop and show a non-destructive preview for manual review.
5. **Restore** from a selected backup by writing it atomically. Keep backups until the user removes them.
6. **Verify delivery** only after the receiver observes actual data. A successful config write or receiver health check alone must never be shown as “Codex connected”. Tell the user to restart Codex manually; do not restart it automatically.

The current receiver binds to loopback and has no OTLP authentication token yet. Until Codex Desktop header support has been validated, retain loopback-only binding and never configure a non-loopback endpoint.

The Flutter settings page now calls authenticated local API routes for detect/preview/apply/restore. The API uses the random token and dynamic port from the Flutter/backend startup handshake; config mutation is not exposed on the OTLP listener. TOML edits use the comment-preserving `go-toml` document editor pinned in `backend/go.mod`. Its editing package is explicitly unstable, so the pinned version must be reviewed when upgrading. Unsupported or invalid config is left untouched and shown as a safe error.
