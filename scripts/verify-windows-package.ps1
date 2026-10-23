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
$testRoot = [IO.Path]::GetFullPath((Join-Path $tempRoot ('AgentPulse-package-smoke-' + [Guid]::NewGuid().ToString('N'))))
if (-not $testRoot.StartsWith($tempRoot + 'AgentPulse-package-smoke-', [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to use a test directory outside the dedicated temporary prefix: $testRoot"
}

$originalLocalAppData = $env:LOCALAPPDATA
$originalAppData = $env:APPDATA
$originalHttpProxy = $env:HTTP_PROXY
$originalHttpsProxy = $env:HTTPS_PROXY
$originalNoProxy = $env:NO_PROXY
$trackedProcesses = [Collections.Generic.List[Diagnostics.Process]]::new()

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

function Get-AgentPulseChild([int]$ParentProcessId, [string]$BackendPath) {
    return Get-CimInstance Win32_Process -Filter "ParentProcessId = $ParentProcessId" |
        Where-Object { $_.Name -eq 'agentpulse-backend.exe' -and $_.ExecutablePath -ieq $BackendPath } |
        Select-Object -First 1
}

function Test-LoopbackPort([int]$Port) {
    $client = [Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.ConnectAsync('127.0.0.1', $Port)
        return $connect.Wait(500) -and $client.Connected
    }
    catch {
        return $false
    }
    finally {
        $client.Dispose()
    }
}

function Wait-DesktopBackend([Diagnostics.Process]$DesktopProcess, [string]$BackendPath, [int]$Port) {
    $deadline = [DateTime]::UtcNow.AddSeconds(25)
    do {
        $DesktopProcess.Refresh()
        if ($DesktopProcess.HasExited) { throw "Desktop exited during startup with code $($DesktopProcess.ExitCode)." }
        $child = Get-AgentPulseChild $DesktopProcess.Id $BackendPath
        if ($child -and (Test-LoopbackPort $Port)) {
            $backendProcess = [Diagnostics.Process]::GetProcessById([int]$child.ProcessId)
            $trackedProcesses.Add($backendProcess)
            return $backendProcess
        }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Packaged desktop did not start its receiver on 127.0.0.1:$Port."
}

try {
    New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
    $bundleDirectory = Join-Path $testRoot 'bundle'
    Expand-Archive -LiteralPath $package -DestinationPath $bundleDirectory

    $env:LOCALAPPDATA = Join-Path $testRoot 'local'
    $env:APPDATA = Join-Path $testRoot 'roaming'
    New-Item -ItemType Directory -Path $env:LOCALAPPDATA,$env:APPDATA -Force | Out-Null

    $installer = Join-Path $bundleDirectory 'Install-AgentPulse.ps1'
    $installDirectory = Join-Path $env:LOCALAPPDATA 'Programs\AgentPulse'
    $userDataDirectory = Join-Path $env:APPDATA 'AgentPulse'
    & $installer -NoShortcut
    if (-not (Test-Path -LiteralPath (Join-Path $installDirectory 'agentpulse.exe') -PathType Leaf)) {
        throw 'Install did not place the desktop application.'
    }

    # Seed a non-default OTLP port through the authenticated settings API in a clean profile.
    # The override lets this isolated setup avoid the user's active receiver.
    $testDatabase = Join-Path $userDataDirectory 'agentpulse.db'
    $backendExecutable = Join-Path $installDirectory 'agentpulse-backend.exe'
    $setupBackend = Start-TestBackend $backendExecutable $testDatabase '127.0.0.1:0'
    $portProbe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $portProbe.Start()
    $customPort = ([Net.IPEndPoint]$portProbe.LocalEndpoint).Port
    $portProbe.Stop()
    $updateBody = @{ port = $customPort } | ConvertTo-Json -Compress
    $update = Invoke-RestMethod -Method Put `
        -Uri "http://$($setupBackend.Handshake.api_address)/api/v1/settings/receiver" `
        -Headers @{ Authorization = "Bearer $($setupBackend.Handshake.api_token)" } `
        -ContentType 'application/json' -Body $updateBody
    if ($update.settings.port -ne $customPort -or -not $update.restart_required) {
        throw 'Receiver settings API did not persist the selected port.'
    }
    Stop-TestBackend $setupBackend

    $customBackend = Start-TestBackend $backendExecutable $testDatabase $null
    if ($customBackend.Handshake.otlp_address -ne "127.0.0.1:$customPort") {
        throw "Backend did not restart on the saved port: $($customBackend.Handshake.otlp_address)."
    }
    Stop-TestBackend $customBackend

    # Launch the installed UI with only the isolated profile and a disconnected proxy.
    # Localhost remains exempt so the UI can communicate with its child receiver.
    $env:HTTP_PROXY = 'http://127.0.0.1:9'
    $env:HTTPS_PROXY = 'http://127.0.0.1:9'
    $env:NO_PROXY = '127.0.0.1,localhost'
    $desktopExecutable = Join-Path $installDirectory 'agentpulse.exe'
    $desktop = Start-Process -FilePath $desktopExecutable -PassThru -WindowStyle Hidden
    $trackedProcesses.Add($desktop)
    $desktopBackend = Wait-DesktopBackend $desktop $backendExecutable $customPort
    $desktop.Kill()
    if (-not $desktop.WaitForExit(10000)) { throw 'Desktop process did not terminate after forced close.' }
    if (-not $desktopBackend.WaitForExit(15000)) {
        throw 'Backend did not exit after forced desktop termination.'
    }
    if (Test-LoopbackPort $customPort) {
        throw 'Custom receiver port remained open after forced desktop termination.'
    }

    $restartedDesktop = Start-Process -FilePath $desktopExecutable -PassThru -WindowStyle Hidden
    $trackedProcesses.Add($restartedDesktop)
    $restartedBackend = Wait-DesktopBackend $restartedDesktop $backendExecutable $customPort
    $restartedDesktop.Kill()
    if (-not $restartedDesktop.WaitForExit(10000)) { throw 'Recovered desktop process did not terminate.' }
    if (-not $restartedBackend.WaitForExit(15000)) {
        throw 'Recovered backend did not exit after desktop termination.'
    }
    if (Test-LoopbackPort $customPort) {
        throw 'Custom receiver port remained open after recovery smoke cleanup.'
    }
    New-Item -ItemType Directory -Path $userDataDirectory -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $userDataDirectory 'preserve.marker') -Value 'keep'
    Set-Content -LiteralPath (Join-Path $installDirectory 'old-release.marker') -Value 'replace'
    & $installer -NoShortcut
    if (Test-Path -LiteralPath (Join-Path $installDirectory 'old-release.marker')) {
        throw 'Upgrade did not replace the prior application files.'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $userDataDirectory 'preserve.marker') -PathType Leaf)) {
        throw 'Upgrade removed user data.'
    }

    $startInfo = [Diagnostics.ProcessStartInfo]::new((Join-Path $installDirectory 'agentpulse-backend.exe'))
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.CreateNoWindow = $true
    $startInfo.ArgumentList.Add('--handshake-stdout')
    $startInfo.ArgumentList.Add('--otlp-address')
    $startInfo.ArgumentList.Add('127.0.0.1:0')
    $startInfo.ArgumentList.Add('--db')
    $startInfo.ArgumentList.Add((Join-Path $testRoot 'shutdown-test.db'))
    $backend = [Diagnostics.Process]::Start($startInfo)
    $handshake = $backend.StandardOutput.ReadLine()
    if (-not $handshake -or $handshake -notmatch '"api_version":1') {
        throw 'Backend did not emit its startup handshake.'
    }
    $backend.StandardInput.Close()
    if (-not $backend.WaitForExit(10000)) {
        $backend.Kill()
        throw 'Backend did not stop after its parent closed the local pipe.'
    }
    if ($backend.ExitCode -ne 0) {
        throw "Backend shutdown returned exit code $($backend.ExitCode)."
    }

    & (Join-Path $installDirectory 'Uninstall-AgentPulse.ps1')
    if (Test-Path -LiteralPath $installDirectory) {
        throw 'Uninstall left the application directory behind.'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $userDataDirectory 'preserve.marker') -PathType Leaf)) {
        throw 'Default uninstall removed user data.'
    }

    & $installer -NoShortcut
    & (Join-Path $installDirectory 'Uninstall-AgentPulse.ps1') -RemoveUserData
    if (Test-Path -LiteralPath $userDataDirectory) {
        throw 'Explicit data removal left the user data directory behind.'
    }

    Write-Output 'Windows package install, upgrade, uninstall, data preservation/removal, custom receiver port, forced desktop termination, and recovery passed under an isolated profile.'
}
finally {
    [Environment]::SetEnvironmentVariable('LOCALAPPDATA', $originalLocalAppData, 'Process')
    [Environment]::SetEnvironmentVariable('APPDATA', $originalAppData, 'Process')
    [Environment]::SetEnvironmentVariable('HTTP_PROXY', $originalHttpProxy, 'Process')
    [Environment]::SetEnvironmentVariable('HTTPS_PROXY', $originalHttpsProxy, 'Process')
    [Environment]::SetEnvironmentVariable('NO_PROXY', $originalNoProxy, 'Process')
    foreach ($process in $trackedProcesses) {
        try {
            $process.Refresh()
            if (-not $process.HasExited) { $process.Kill() }
        }
        catch { }
    }
    if (Test-Path -LiteralPath $testRoot) {
        $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
        $expectedPrefix = [IO.Path]::GetFullPath($tempRoot + 'AgentPulse-package-smoke-')
        if ($resolvedTestRoot.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase) -and
            (Split-Path -Leaf $resolvedTestRoot).StartsWith('AgentPulse-package-smoke-', [StringComparison]::OrdinalIgnoreCase)) {
            Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
        }
    }
}
