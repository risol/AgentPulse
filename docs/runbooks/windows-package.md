# Windows package and recovery runbook

## Build and verify

From the repository root in PowerShell:

```powershell
.\scripts\build-windows.ps1 -BuildDesktop
.\scripts\verify-windows-package.ps1 -PackagePath .\dist\AgentPulse-Windows-x64.zip
```

The build writes the offline bundle to `dist\AgentPulse-Windows-x64.zip`. The verifier installs it under an isolated temporary profile and removes that profile after checking installation, upgrade, uninstall, receiver-port persistence, forced shutdown, and backend recovery. The smoke verifies the package lifecycle; it does not apply an operating-system network block or prove that all OS networking is unavailable.

## Install or upgrade

1. Extract the ZIP to a writable directory.
2. Close AgentPulse before upgrading.
3. Run `Install-AgentPulse.ps1` from the extracted directory.
4. Start AgentPulse from the Start menu.

The app installs under `%LOCALAPPDATA%\Programs\AgentPulse`. Upgrades replace application files and preserve the separate database, logs, and Codex configuration.

## Check collection health

The default OTLP health endpoint is `http://127.0.0.1:4318/health`. A healthy response includes `"status":"ready"` and the current `schema_version`. The receiver port can be changed from Settings; after a change, use the endpoint shown in the Codex configuration preview.

The application does not restart Codex. If it was configured while Codex was open, restart Codex from its normal UI and confirm that recent telemetry arrives before treating collection as healthy.

## Preserve or remove data

Use Settings to export a consistent database snapshot before a major upgrade or removal if the history may be needed. Normal uninstall removes the application and shortcuts but preserves `%APPDATA%\AgentPulse`. To remove the data too, run the installed `Uninstall-AgentPulse.ps1` with `-RemoveUserData`; that deletes the database and local application logs.

If the receiver cannot start because its port is occupied, identify which local application owns the configured port before changing or stopping it. The AgentPulse UI reports the conflict and supports selecting another loopback port.

## Network isolation status

The ZIP includes the application runtime and has no installer-time network dependency. The package smoke used a disconnected proxy with loopback exempt; it did not impose an operating-system network block. The user waived strict OS-level offline isolation on 2026-10-23. This check was not performed and is not recorded as a technical pass. If a future release specifically requires strict isolation evidence, run it in a disposable Windows environment or with a narrowly scoped temporary firewall rule.
