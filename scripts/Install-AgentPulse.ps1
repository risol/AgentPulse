[CmdletBinding()]
param(
    [switch]$NoShortcut
)

$ErrorActionPreference = 'Stop'
$sourceDirectory = [IO.Path]::GetFullPath((Split-Path -Parent $MyInvocation.MyCommand.Path))
$InstallDirectory = [IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'Programs\AgentPulse'))
$requiredFiles = @(
    'agentpulse.exe',
    'agentpulse-launcher.exe',
    'agentpulse-backend.exe',
    'flutter_windows.dll',
    'data\app.so',
    'Uninstall-AgentPulse.ps1'
)

foreach ($relativePath in $requiredFiles) {
    if (-not (Test-Path -LiteralPath (Join-Path $sourceDirectory $relativePath) -PathType Leaf)) {
        throw "The installation bundle is incomplete. Missing: $relativePath"
    }
}

$runningProcess = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Where-Object {
        if ($_.Name -notin @('agentpulse-launcher.exe', 'agentpulse.exe') -or -not $_.ExecutablePath) { return $false }
        $processPath = [IO.Path]::GetFullPath($_.ExecutablePath)
        $processPath.Equals((Join-Path $InstallDirectory 'agentpulse-launcher.exe'), [StringComparison]::OrdinalIgnoreCase) -or
            $processPath.Equals((Join-Path $InstallDirectory 'agentpulse.exe'), [StringComparison]::OrdinalIgnoreCase)
    } |
    Select-Object -First 1
if ($runningProcess) {
    throw 'Close AgentPulse before installing or upgrading it. Your collection data will be preserved.'
}

$installParent = Split-Path -Parent $InstallDirectory
$installName = Split-Path -Leaf $InstallDirectory
New-Item -ItemType Directory -Path $installParent -Force | Out-Null
$stageDirectory = Join-Path $installParent (".$installName.installing-" + [Guid]::NewGuid().ToString('N'))
$backupDirectory = Join-Path $installParent (".$installName.previous-" + [Guid]::NewGuid().ToString('N'))
$movedPrevious = $false

try {
    New-Item -ItemType Directory -Path $stageDirectory -Force | Out-Null
    Get-ChildItem -LiteralPath $sourceDirectory -Force | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $stageDirectory -Recurse -Force
    }
    foreach ($relativePath in $requiredFiles) {
        if (-not (Test-Path -LiteralPath (Join-Path $stageDirectory $relativePath) -PathType Leaf)) {
            throw "The staged installation is incomplete. Missing: $relativePath"
        }
    }

    if (Test-Path -LiteralPath $InstallDirectory) {
        Move-Item -LiteralPath $InstallDirectory -Destination $backupDirectory
        $movedPrevious = $true
    }
    try {
        Move-Item -LiteralPath $stageDirectory -Destination $InstallDirectory
    }
    catch {
        if ($movedPrevious -and -not (Test-Path -LiteralPath $InstallDirectory)) {
            Move-Item -LiteralPath $backupDirectory -Destination $InstallDirectory
            $movedPrevious = $false
        }
        throw
    }

    if ($movedPrevious) {
        Remove-Item -LiteralPath $backupDirectory -Recurse -Force
        $movedPrevious = $false
    }

    if (-not $NoShortcut) {
        $shortcutDirectory = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\AgentPulse'
        New-Item -ItemType Directory -Path $shortcutDirectory -Force | Out-Null
        $shell = New-Object -ComObject WScript.Shell

        $appShortcut = $shell.CreateShortcut((Join-Path $shortcutDirectory 'AgentPulse.lnk'))
        $appShortcut.TargetPath = Join-Path $InstallDirectory 'agentpulse-launcher.exe'
        $appShortcut.Arguments = '--launch-desktop'
        $appShortcut.IconLocation = Join-Path $InstallDirectory 'agentpulse.exe'
        $appShortcut.WorkingDirectory = $InstallDirectory
        $appShortcut.Save()

        $uninstallShortcut = $shell.CreateShortcut((Join-Path $shortcutDirectory 'Uninstall AgentPulse.lnk'))
        $uninstallShortcut.TargetPath = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
        $uninstallShortcut.Arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $InstallDirectory 'Uninstall-AgentPulse.ps1') + '"'
        $uninstallShortcut.WorkingDirectory = $InstallDirectory
        $uninstallShortcut.Save()
    }

    Write-Output "AgentPulse is installed at $InstallDirectory. Existing collection data and Codex settings are preserved."
}
catch {
    if (Test-Path -LiteralPath $stageDirectory) {
        Remove-Item -LiteralPath $stageDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
    if ($movedPrevious -and -not (Test-Path -LiteralPath $InstallDirectory) -and (Test-Path -LiteralPath $backupDirectory)) {
        Move-Item -LiteralPath $backupDirectory -Destination $InstallDirectory -ErrorAction SilentlyContinue
    }
    throw
}
