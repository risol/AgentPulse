[CmdletBinding()]
param(
    [string]$PackagePath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\dist\AgentPulse-Windows-x64.zip'))
)

$ErrorActionPreference = 'Stop'
$package = [IO.Path]::GetFullPath($PackagePath)
if (-not (Test-Path -LiteralPath $package -PathType Leaf)) {
    throw "Windows package not found: $package"
}

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
$testRoot = [IO.Path]::GetFullPath((Join-Path $tempRoot ('AgentPulse-e2e-' + [Guid]::NewGuid().ToString('N'))))
if (-not $testRoot.StartsWith($tempRoot + 'AgentPulse-e2e-', [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to use a test directory outside the dedicated temporary prefix: $testRoot"
}
$visualReviewRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\dist\e2e-visual-review')).TrimEnd('\') + '\'
$reviewRunID = (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N')
$screenshotDirectory = [IO.Path]::GetFullPath((Join-Path $visualReviewRoot $reviewRunID))
if (-not $screenshotDirectory.StartsWith($visualReviewRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to write E2E screenshots outside the review output directory: $screenshotDirectory"
}

$originalLocalAppData = $env:LOCALAPPDATA
$originalAppData = $env:APPDATA
$originalBackend = $env:AGENTPULSE_BACKEND
$originalAddress = $env:AGENTPULSE_E2E_OTLP_ADDRESS
$originalResult = $env:AGENTPULSE_E2E_RESULT
$originalProcessRoot = $env:AGENTPULSE_E2E_PROCESS_ROOT
$originalScreenshotDirectory = $env:AGENTPULSE_E2E_SCREENSHOT_DIR
$originalScreenshotHelper = $env:AGENTPULSE_E2E_SCREENSHOT_HELPER
$trackedProcesses = [Collections.Generic.List[Diagnostics.Process]]::new()
$existingCodexPids = @(
    Get-CimInstance Win32_Process -Filter "name='codex.exe'" |
        Select-Object -ExpandProperty ProcessId
)
$processRootName = if ($existingCodexPids.Count -gt 0) { 'codex.exe' } else { 'agentpulse.exe' }

function Start-TestBackend([string]$Executable, [string]$DatabasePath, [string]$AddressOverride) {
    $startInfo = [Diagnostics.ProcessStartInfo]::new($Executable)
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.CreateNoWindow = $true
    $startInfo.ArgumentList.Add('--handshake-stdout')
    $startInfo.ArgumentList.Add('--db')
    $startInfo.ArgumentList.Add($DatabasePath)
    if ($AddressOverride) {
        $startInfo.ArgumentList.Add('--otlp-address')
        $startInfo.ArgumentList.Add($AddressOverride)
    }
    $process = [Diagnostics.Process]::Start($startInfo)
    $trackedProcesses.Add($process)
    $handshakeLine = $process.StandardOutput.ReadLineAsync()
    if (-not $handshakeLine.Wait(15000)) {
        if (-not $process.HasExited) { $process.Kill() }
        throw 'Backend did not emit its startup handshake within 15 seconds.'
    }
    try {
        $handshake = $handshakeLine.Result | ConvertFrom-Json
    }
    catch {
        $errorText = $process.StandardError.ReadToEnd()
        throw "Backend emitted an invalid startup handshake. $errorText"
    }
    if (-not $handshake.api_address -or -not $handshake.api_token -or -not $handshake.otlp_address) {
        throw 'Backend startup handshake is missing required addresses or credentials.'
    }
    return [pscustomobject]@{ Process = $process; Handshake = $handshake }
}

function Stop-TestBackend($Backend) {
    if (-not $Backend.Process.HasExited) {
        $Backend.Process.StandardInput.Close()
        if (-not $Backend.Process.WaitForExit(10000)) {
            $Backend.Process.Kill()
            throw 'Backend did not stop after its parent pipe was closed.'
        }
    }
    if ($Backend.Process.ExitCode -ne 0) {
        throw "Backend shutdown returned exit code $($Backend.Process.ExitCode)."
    }
}

try {
    New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
    $bundleDirectory = Join-Path $testRoot 'bundle'
    Expand-Archive -LiteralPath $package -DestinationPath $bundleDirectory

    $env:LOCALAPPDATA = Join-Path $testRoot 'local'
    $env:APPDATA = Join-Path $testRoot 'roaming'
    New-Item -ItemType Directory -Path $env:LOCALAPPDATA,$env:APPDATA -Force | Out-Null
    $userDataDirectory = Join-Path $env:APPDATA 'AgentPulse'
    New-Item -ItemType Directory -Path $userDataDirectory -Force | Out-Null

    $backendExecutable = Join-Path $bundleDirectory 'agentpulse-backend.exe'
    $testDatabase = Join-Path $userDataDirectory 'agentpulse.db'
    $setupBackend = Start-TestBackend $backendExecutable $testDatabase '127.0.0.1:0'
    $portProbe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $portProbe.Start()
    $customPort = ([Net.IPEndPoint]$portProbe.LocalEndpoint).Port
    $portProbe.Stop()
    $update = Invoke-RestMethod -Method Put `
        -Uri "http://$($setupBackend.Handshake.api_address)/api/v1/settings/receiver" `
        -Headers @{ Authorization = "Bearer $($setupBackend.Handshake.api_token)" } `
        -ContentType 'application/json' -Body (@{ port = $customPort } | ConvertTo-Json -Compress)
    if ($update.settings.port -ne $customPort -or -not $update.restart_required) {
        throw 'Receiver settings API did not persist the isolated test port.'
    }
    Stop-TestBackend $setupBackend

    $env:AGENTPULSE_BACKEND = $backendExecutable
    $env:AGENTPULSE_E2E_OTLP_ADDRESS = "127.0.0.1:$customPort"
    $env:AGENTPULSE_E2E_RESULT = Join-Path $testRoot 'e2e-result.json'
    $env:AGENTPULSE_E2E_PROCESS_ROOT = $processRootName
    $env:AGENTPULSE_E2E_SCREENSHOT_DIR = $screenshotDirectory
    $env:AGENTPULSE_E2E_SCREENSHOT_HELPER = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'capture-windows-e2e-window.ps1'))
    New-Item -ItemType Directory -Path $screenshotDirectory -Force | Out-Null
    Push-Location (Join-Path $PSScriptRoot '..\desktop')
    try {
        & flutter test integration_test/agentpulse_e2e_test.dart -d windows
        if ($LASTEXITCODE -ne 0) {
            throw "Flutter Windows end-to-end test failed with exit code $LASTEXITCODE."
        }
    }
    finally {
        Pop-Location
    }

    $resultPath = $env:AGENTPULSE_E2E_RESULT
    if (-not (Test-Path -LiteralPath $resultPath -PathType Leaf)) {
        throw 'The UI test did not write its latency report.'
    }
    $expectedScreenshots = @('dashboard', 'process-settings', 'process-settings-details', 'process-monitoring', 'session-detail', 'session-process-tree', 'tool-analysis')
    foreach ($screenshotName in $expectedScreenshots) {
        $screenshotPath = Join-Path $screenshotDirectory ($screenshotName + '.png')
        if (-not (Test-Path -LiteralPath $screenshotPath -PathType Leaf)) {
            throw "The UI test did not capture its '$screenshotName' screenshot."
        }
    }
    $uiResult = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json

    $verifyBackend = Start-TestBackend $backendExecutable $testDatabase '127.0.0.1:0'
    try {
        $summary = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/dashboard/summary?range=1h" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $databaseLogCount = [int]$summary.events.logs
        $monitorSettings = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/settings/process-monitor" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $processPage = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/processes?limit=500" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $sessionList = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/sessions?range=all&limit=25" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $workflowSession = @($sessionList.items | Where-Object { $_.external_id -eq [string]$uiResult.workflow_session_external_id }) | Select-Object -First 1
        if (-not $workflowSession) {
            throw "The workflow session '$($uiResult.workflow_session_external_id)' was not persisted."
        }
        $encodedSessionId = [Uri]::EscapeDataString([string]$workflowSession.id)
        $sessionTimeline = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/sessions/$encodedSessionId/timeline?limit=100" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $sessionProcesses = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/sessions/$encodedSessionId/processes" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $toolCalls = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/tools/calls?range=24h&limit=25&session_id=$encodedSessionId" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
        $sessionAnalysis = Invoke-RestMethod -Method Get `
            -Uri "http://$($verifyBackend.Handshake.api_address)/api/v1/sessions/$encodedSessionId/analysis" `
            -Headers @{ Authorization = "Bearer $($verifyBackend.Handshake.api_token)" }
    }
    finally {
        Stop-TestBackend $verifyBackend
    }

    if ($uiResult.event_count -ne 20 -or $uiResult.dashboard_log_count -ne 20 -or $databaseLogCount -ne 20) {
        throw "UI/database counts disagree: sent=$($uiResult.event_count), UI=$($uiResult.dashboard_log_count), persisted=$databaseLogCount."
    }
    if ($uiResult.telemetry_to_dashboard_p95_ms -ge 1000) {
        throw "Telemetry-to-dashboard P95 was $($uiResult.telemetry_to_dashboard_p95_ms) ms; target is below 1000 ms."
    }
    $processConfig = $monitorSettings.config
    if ($processConfig.enabled -or $processConfig.sample_interval_ms -ne 500 -or
        $processConfig.capture_executable_paths -or $processConfig.capture_command_lines -or
        $processConfig.root_process_names.Count -ne 1 -or
        $processConfig.root_process_names[0] -ine $processRootName) {
        throw 'The UI did not leave process monitoring disabled with the expected 500 ms privacy-preserving settings.'
    }
    $matchingProcesses = @($processPage.items | Where-Object { [string]$_.image_name -ieq $processRootName })
    if ($matchingProcesses.Count -eq 0 -or
        -not $uiResult.process_monitor_enabled_during_sample -or
        -not $uiResult.process_monitor_disabled_after_sample -or
        -not $uiResult.process_monitor_sampling_status_visible) {
        throw "The UI did not show sampling or persist a sampled $processRootName record."
    }
    if (@($matchingProcesses | Where-Object { $_.executable_path -or $_.command_line }).Count -gt 0) {
        throw 'The process sample unexpectedly persisted an executable path or command line.'
    }
    if ($processRootName -eq 'codex.exe' -and
        @($matchingProcesses | Where-Object { $existingCodexPids -contains [int]$_.pid }).Count -eq 0) {
        throw 'The persisted Codex root records did not include a codex.exe process present during the run.'
    }

    $workflowTools = @($toolCalls.items)
    $workflowTimelineTools = @($sessionTimeline.items | Where-Object { $_.kind -eq 'tool_call' })
    if ($workflowTools.Count -ne 3 -or $workflowTimelineTools.Count -ne 3 -or
        $uiResult.workflow_ui_tool_rows -ne $workflowTimelineTools.Count) {
        throw "Workflow UI/API/database tool counts disagree: UI=$($uiResult.workflow_ui_tool_rows), timeline=$($workflowTimelineTools.Count), calls=$($workflowTools.Count)."
    }
    if ($workflowSession.tool_call_count -ne 3 -or $workflowSession.command_count -ne 2 -or
        $workflowSession.failed_call_count -ne 1 -or $sessionAnalysis.tools.Count -ne 2) {
        throw 'Workflow session summary did not preserve its three calls, two commands, and one expected failure.'
    }
    foreach ($expectedSpanId in @($uiResult.workflow_span_ids)) {
        if (@($workflowTimelineTools | Where-Object { $_.span_id -eq [string]$expectedSpanId }).Count -ne 1) {
            throw "Workflow span $expectedSpanId is missing or duplicated in the persisted timeline."
        }
    }
    $successCall = @($workflowTools | Where-Object { $_.external_id -eq [string]$uiResult.workflow_tool_call_ids[0] })
    $failureCall = @($workflowTools | Where-Object { $_.external_id -eq [string]$uiResult.workflow_tool_call_ids[1] })
    $testCall = @($workflowTools | Where-Object { $_.external_id -eq [string]$uiResult.workflow_tool_call_ids[2] })
    if ($successCall.Count -ne 1 -or $successCall[0].status -ne 'success' -or
        $failureCall.Count -ne 1 -or $failureCall[0].status -ne 'error' -or
        $testCall.Count -ne 1 -or $testCall[0].status -ne 'success') {
        throw 'The successful command, expected failure, or test-program outcome was not persisted correctly.'
    }
    if ($successCall[0].tool_type -ne 'command' -or $failureCall[0].tool_type -ne 'command' -or
        $testCall[0].tool_type -ne 'test' -or $failureCall[0].retry_index -ne 1) {
        throw 'Tool types or the retry index were not preserved in the persisted workflow calls.'
    }
    if (@($workflowTools | Where-Object { $_.duration_ms -lt 1500 }).Count -gt 0) {
        throw 'At least one fixed workflow process ran for less than the required 1.5 seconds.'
    }
    $associatedProcesses = @($sessionProcesses.items | Where-Object { $_.association_method -eq 'explicit_process_pid' })
    foreach ($expectedPid in @($uiResult.workflow_process_pids)) {
        if (@($associatedProcesses | Where-Object { [int]$_.pid -eq [int]$expectedPid -and $_.image_name -ieq 'powershell.exe' }).Count -ne 1) {
            throw "Workflow process PID $expectedPid has no explicit persisted session association."
        }
    }
    if (@($associatedProcesses | Where-Object { $_.executable_path -or $_.command_line }).Count -gt 0) {
        throw 'A workflow process association unexpectedly persisted a path or command line.'
    }

    [pscustomobject]@{
        Workflow = 'Windows desktop UI + successful/failed commands + test program + OTLP spans'
        EventsSent = $uiResult.event_count
        DashboardCount = $uiResult.dashboard_log_count
        PersistedCount = $databaseLogCount
        TelemetryToDashboardP95MS = $uiResult.telemetry_to_dashboard_p95_ms
        SamplesMS = $uiResult.telemetry_to_dashboard_samples_ms -join ', '
        ProcessRoot = $processRootName
        ProcessRecordsPersisted = $matchingProcesses.Count
        SessionExternalID = $workflowSession.external_id
        TimelineToolSpans = $workflowTimelineTools.Count
        ToolCalls = $workflowTools.Count
        FailedCalls = $workflowSession.failed_call_count
        ExplicitProcessAssociations = $associatedProcesses.Count
        WorkflowProcessPIDs = $uiResult.workflow_process_pids -join ', '
        ProcessMonitorDisabledAfterRun = -not $processConfig.enabled
        PathsAndCommandLinesPersisted = $false
        VisualReviewDirectory = $screenshotDirectory
        VisualReviewScreenshots = $expectedScreenshots.Count
    } | Format-List
}
finally {
    [Environment]::SetEnvironmentVariable('LOCALAPPDATA', $originalLocalAppData, 'Process')
    [Environment]::SetEnvironmentVariable('APPDATA', $originalAppData, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_BACKEND', $originalBackend, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_E2E_OTLP_ADDRESS', $originalAddress, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_E2E_RESULT', $originalResult, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_E2E_PROCESS_ROOT', $originalProcessRoot, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_E2E_SCREENSHOT_DIR', $originalScreenshotDirectory, 'Process')
    [Environment]::SetEnvironmentVariable('AGENTPULSE_E2E_SCREENSHOT_HELPER', $originalScreenshotHelper, 'Process')
    foreach ($process in $trackedProcesses) {
        try {
            $process.Refresh()
            if (-not $process.HasExited) { $process.Kill() }
        }
        catch { }
    }
    if (Test-Path -LiteralPath $testRoot) {
        $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
        $expectedPrefix = [IO.Path]::GetFullPath($tempRoot + 'AgentPulse-e2e-')
        if ($resolvedTestRoot.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase) -and
            (Split-Path -Leaf $resolvedTestRoot).StartsWith('AgentPulse-e2e-', [StringComparison]::OrdinalIgnoreCase)) {
            Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
        }
    }
}
