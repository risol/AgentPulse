$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$desktopLib = [IO.Path]::GetFullPath((Join-Path $repoRoot 'desktop\lib'))
$architectureErrors = [Collections.Generic.List[string]]::new()

function Test-PathWithin {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Directory
    )

    $resolvedPath = [IO.Path]::GetFullPath($Path)
    $resolvedDirectory = [IO.Path]::GetFullPath($Directory).TrimEnd('\') + '\'
    return $resolvedPath.StartsWith($resolvedDirectory, [StringComparison]::OrdinalIgnoreCase)
}

Push-Location $backendDir
try {
    $modulePath = (& go list -m).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $modulePath) {
        throw 'Could not read the Go module path.'
    }

    $packageLines = & go list -f '{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}' ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not inspect Go package dependencies.'
    }

    $restrictedDependencies = @(
        @{
            Package = "$modulePath/internal/application"
            Forbidden = @(
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexconfig",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/ingest",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/analytics"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexadapter",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/correlation"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexadapter",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/process"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/ingest",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/telemetry"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexadapter",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/codexadapter"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/analytics",
                "$modulePath/internal/correlation",
                "$modulePath/internal/process",
                "$modulePath/internal/ingest",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/ingest"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/storage",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/analytics",
                "$modulePath/internal/correlation",
                "$modulePath/internal/process",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/otlp"
            Forbidden = @(
                "$modulePath/internal/analytics",
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/correlation",
                "$modulePath/internal/process",
                "$modulePath/internal/storage",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/storage"
            Forbidden = @(
                "$modulePath/internal/application",
                "$modulePath/internal/appapi",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/ingest",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        },
        @{
            Package = "$modulePath/internal/appapi"
            Forbidden = @(
                "$modulePath/internal/storage",
                "$modulePath/internal/codexconfig",
                "$modulePath/internal/codexadapter",
                "$modulePath/internal/ingest",
                "$modulePath/internal/otlp",
                "$modulePath/cmd/agentpulse"
            )
        }
    )

    foreach ($line in $packageLines) {
        $parts = $line -split '\|', 2
        if ($parts.Count -ne 2) { continue }
        $package = $parts[0]
        $imports = @($parts[1] -split '\s+' | Where-Object { $_ })
        $rule = $restrictedDependencies | Where-Object {
            $package -eq $_.Package -or $package.StartsWith("$($_.Package)/")
        } | Select-Object -First 1
        if (-not $rule) { continue }

        foreach ($import in $imports) {
            foreach ($forbidden in $rule.Forbidden) {
                if ($import -eq $forbidden -or $import.StartsWith("$forbidden/")) {
                    $architectureErrors.Add("Go dependency points inward: $package imports $import")
                }
            }
        }
    }
}
finally {
    Pop-Location
}

$coreDir = Join-Path $desktopLib 'core'
$featuresDir = Join-Path $desktopLib 'features'
$featuresPrefix = [IO.Path]::GetFullPath($featuresDir).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
$directivePattern = '(?m)^\s*(?:import|export)\s+(?:[^;]*?\s+from\s+)?[''"]([^''"]+)[''"]'
foreach ($file in Get-ChildItem -LiteralPath $desktopLib -Recurse -File -Filter '*.dart') {
    $sourcePath = [IO.Path]::GetFullPath($file.FullName)
    $sourceText = Get-Content -LiteralPath $sourcePath -Raw
    foreach ($match in [regex]::Matches($sourceText, $directivePattern)) {
        $reference = $match.Groups[1].Value
        if ($reference.StartsWith('package:agentpulse/')) {
            $targetPath = Join-Path $desktopLib ($reference.Substring('package:agentpulse/'.Length).Replace('/', '\'))
        } elseif ($reference.StartsWith('package:') -or $reference.StartsWith('dart:')) {
            continue
        } else {
            $targetPath = Join-Path $file.DirectoryName $reference
        }

        $targetPath = [IO.Path]::GetFullPath($targetPath)
        if ((Test-PathWithin -Path $sourcePath -Directory $coreDir) -and
            (Test-PathWithin -Path $targetPath -Directory $featuresDir)) {
            $architectureErrors.Add("Flutter core imports a feature: $($file.FullName) -> $reference")
        }
        if ((Test-PathWithin -Path $sourcePath -Directory $featuresDir) -and
            $targetPath -eq [IO.Path]::GetFullPath((Join-Path $desktopLib 'main.dart'))) {
            $architectureErrors.Add("Flutter feature imports the composition root: $($file.FullName)")
        }
        if ((Test-PathWithin -Path $sourcePath -Directory $featuresDir) -and
            (Test-PathWithin -Path $targetPath -Directory $featuresDir)) {
            $sourceFeature = $sourcePath.Substring($featuresPrefix.Length) -split '[\\/]'
            $targetFeature = $targetPath.Substring($featuresPrefix.Length) -split '[\\/]'
            $sourceFeature = $sourceFeature[0]
            $targetFeature = $targetFeature[0]
            if ($sourceFeature -ne $targetFeature) {
                $architectureErrors.Add("Flutter feature imports another feature: $($file.FullName) -> $reference")
            }
        }
    }
}

if ($architectureErrors.Count -gt 0) {
    $architectureErrors | ForEach-Object { Write-Error $_ }
    exit 1
}

Write-Output 'Architecture dependency boundaries passed.'
