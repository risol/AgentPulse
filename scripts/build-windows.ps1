param(
    [switch]$BuildDesktop,
    [string]$OutputDirectory,
    [string]$Version
)
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$desktopManifest = Join-Path $repoRoot 'desktop\pubspec.yaml'
$outputDir = if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    Join-Path $repoRoot 'dist'
} else {
    [IO.Path]::GetFullPath($OutputDirectory)
}
$previousCgoEnabled = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH

New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
$versionLine = Select-String -Path $desktopManifest -Pattern '^version:\s*' | Select-Object -First 1
$manifestVersion = if ($versionLine) { ($versionLine.Line -replace '^version:\s*', '').Trim() } else { 'development' }
$applicationVersion = if ([string]::IsNullOrWhiteSpace($Version)) { $manifestVersion } else { $Version.Trim() }
Push-Location $backendDir
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags "-X main.version=$applicationVersion" -o (Join-Path $outputDir 'agentpulse-backend.exe') ./cmd/agentpulse
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build the Go backend.' }
    go build -trimpath -ldflags "-H=windowsgui -X main.version=$applicationVersion" -o (Join-Path $outputDir 'agentpulse-launcher.exe') ./cmd/agentpulse
    if ($LASTEXITCODE -ne 0) { throw 'Failed to build the Go desktop launcher.' }
}
finally {
    $env:CGO_ENABLED = $previousCgoEnabled
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    Pop-Location
}

if ($BuildDesktop) {
    $desktopReleaseDir = Join-Path $repoRoot 'desktop\build\windows\x64\runner\Release'
    Push-Location (Join-Path $repoRoot 'desktop')
    try {
        if ([string]::IsNullOrWhiteSpace($Version)) {
            flutter build windows --release
        } else {
            $desktopBuildName = $Version.Trim() -replace '^v', ''
            flutter build windows --release --build-name $desktopBuildName
        }
        if (-not (Test-Path -LiteralPath (Join-Path $desktopReleaseDir 'agentpulse.exe'))) {
            throw 'Flutter Windows release output is missing agentpulse.exe.'
        }
        Get-ChildItem -LiteralPath $desktopReleaseDir -Force | ForEach-Object {
            Copy-Item -LiteralPath $_.FullName -Destination $outputDir -Recurse -Force
        }
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\Install-AgentPulse.ps1') -Destination $outputDir -Force
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\Uninstall-AgentPulse.ps1') -Destination $outputDir -Force
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\windows-package-readme.txt') -Destination (Join-Path $outputDir 'README.txt') -Force
		if (-not (Test-Path -LiteralPath (Join-Path $outputDir 'data\app.so')) -or
		    -not (Test-Path -LiteralPath (Join-Path $outputDir 'flutter_windows.dll')) -or
		    -not (Test-Path -LiteralPath (Join-Path $outputDir 'agentpulse-backend.exe')) -or
		    -not (Test-Path -LiteralPath (Join-Path $outputDir 'agentpulse-launcher.exe')) -or
		    -not (Test-Path -LiteralPath (Join-Path $outputDir 'Install-AgentPulse.ps1')) -or
		    -not (Test-Path -LiteralPath (Join-Path $outputDir 'Uninstall-AgentPulse.ps1'))) {
			throw 'Windows distribution is missing required Flutter or backend runtime files.'
		}
		$stageDirectory = Join-Path $outputDir ('.AgentPulse-Windows-x64-stage-' + [Guid]::NewGuid().ToString('N'))
		$archiveTemp = Join-Path $outputDir ('.AgentPulse-Windows-x64-' + [Guid]::NewGuid().ToString('N') + '.zip')
		$archivePath = Join-Path $outputDir 'AgentPulse-Windows-x64.zip'
		$resolvedOutput = [IO.Path]::GetFullPath($outputDir).TrimEnd('\') + '\'
		foreach ($temporaryPath in @($stageDirectory, $archiveTemp)) {
			if (-not [IO.Path]::GetFullPath($temporaryPath).StartsWith($resolvedOutput, [StringComparison]::OrdinalIgnoreCase)) {
				throw "Refusing to use a temporary package path outside dist: $temporaryPath"
			}
		}
		New-Item -ItemType Directory -Path $stageDirectory -Force | Out-Null
		Get-ChildItem -LiteralPath $desktopReleaseDir -Force | ForEach-Object {
			Copy-Item -LiteralPath $_.FullName -Destination $stageDirectory -Recurse -Force
		}
		Copy-Item -LiteralPath (Join-Path $outputDir 'agentpulse-backend.exe') -Destination $stageDirectory -Force
		Copy-Item -LiteralPath (Join-Path $outputDir 'agentpulse-launcher.exe') -Destination $stageDirectory -Force
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\Install-AgentPulse.ps1') -Destination $stageDirectory -Force
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\Uninstall-AgentPulse.ps1') -Destination $stageDirectory -Force
		Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\windows-package-readme.txt') -Destination (Join-Path $stageDirectory 'README.txt') -Force
		if (-not (Test-Path -LiteralPath (Join-Path $stageDirectory 'data\app.so')) -or
		    -not (Test-Path -LiteralPath (Join-Path $stageDirectory 'flutter_windows.dll')) -or
		    -not (Test-Path -LiteralPath (Join-Path $stageDirectory 'agentpulse-backend.exe')) -or
		    -not (Test-Path -LiteralPath (Join-Path $stageDirectory 'agentpulse-launcher.exe'))) {
			throw 'The staged offline bundle is missing runtime files.'
		}
		$archivePath = Join-Path $outputDir 'AgentPulse-Windows-x64.zip'
		Compress-Archive -Path (Join-Path $stageDirectory '*') -DestinationPath $archiveTemp -CompressionLevel Optimal
		Move-Item -LiteralPath $archiveTemp -Destination $archivePath -Force
		Remove-Item -LiteralPath $stageDirectory -Recurse -Force
    }
    finally {
		if ($stageDirectory -and (Test-Path -LiteralPath $stageDirectory)) {
			$resolvedOutput = [IO.Path]::GetFullPath($outputDir).TrimEnd('\') + '\'
			if ([IO.Path]::GetFullPath($stageDirectory).StartsWith($resolvedOutput, [StringComparison]::OrdinalIgnoreCase)) {
				Remove-Item -LiteralPath $stageDirectory -Recurse -Force
			}
		}
		if ($archiveTemp -and (Test-Path -LiteralPath $archiveTemp)) {
			Remove-Item -LiteralPath $archiveTemp -Force
		}
        Pop-Location
    }
}
