[CmdletBinding()]
param(
    [switch]$RemoveUserData
)

$ErrorActionPreference = 'Stop'
$installDirectory = [IO.Path]::GetFullPath((Split-Path -Parent $MyInvocation.MyCommand.Path))
$applicationPath = [IO.Path]::GetFullPath((Join-Path $installDirectory 'agentpulse.exe'))
$runningProcess = Get-CimInstance Win32_Process -Filter "Name = 'agentpulse.exe'" -ErrorAction SilentlyContinue |
    Where-Object {
        $_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath).Equals($applicationPath, [StringComparison]::OrdinalIgnoreCase)
    } |
    Select-Object -First 1
if ($runningProcess) {
    throw 'Close AgentPulse before uninstalling it. Your collection data will be preserved.'
}

$expectedInstallDirectory = [IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'Programs\AgentPulse'))
$expectedInstallParent = [IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'Programs'))
if (-not $installDirectory.Equals($expectedInstallDirectory, [StringComparison]::OrdinalIgnoreCase) -or
    -not $installDirectory.StartsWith($expectedInstallParent + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to remove an unexpected installation directory: $installDirectory"
}

$shortcutDirectory = [IO.Path]::GetFullPath((Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\AgentPulse'))
$expectedShortcutDirectory = [IO.Path]::GetFullPath((Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\AgentPulse'))
if ($shortcutDirectory.Equals($expectedShortcutDirectory, [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $shortcutDirectory)) {
    Remove-Item -LiteralPath $shortcutDirectory -Recurse -Force
}

$userDataDirectory = [IO.Path]::GetFullPath((Join-Path $env:APPDATA 'AgentPulse'))
Remove-Item -LiteralPath $installDirectory -Recurse -Force
if ($RemoveUserData) {
    $expectedUserDataDirectory = [IO.Path]::GetFullPath((Join-Path $env:APPDATA 'AgentPulse'))
    if ($userDataDirectory.Equals($expectedUserDataDirectory, [StringComparison]::OrdinalIgnoreCase) -and
        (Test-Path -LiteralPath $userDataDirectory)) {
        Remove-Item -LiteralPath $userDataDirectory -Recurse -Force
    }
    Write-Output 'AgentPulse and its local collection data were removed.'
}
else {
    Write-Output "AgentPulse was removed. Collection data and settings remain at $userDataDirectory."
}
