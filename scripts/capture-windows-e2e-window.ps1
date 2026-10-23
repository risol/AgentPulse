[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^[a-z0-9][a-z0-9-]{0,63}$')]
    [string]$Name,
    [Parameter(Mandatory)]
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
$output = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Path $output -Force | Out-Null
Add-Type -AssemblyName System.Drawing
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class AgentPulseE2EWindowCapture
{
    [StructLayout(LayoutKind.Sequential)]
    public struct Rect
    {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool GetWindowRect(IntPtr window, out Rect rect);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
}
'@

$previousDpiContext = [AgentPulseE2EWindowCapture]::SetThreadDpiAwarenessContext([IntPtr](-4))
if ($previousDpiContext -eq [IntPtr]::Zero) {
    throw 'Could not enable physical-pixel coordinates for the E2E screenshot.'
}

$window = Get-Process | Where-Object {
    $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle -eq 'agentpulse'
} | Select-Object -First 1
if (-not $window) {
    throw 'The AgentPulse E2E window is not visible.'
}

$rect = [AgentPulseE2EWindowCapture+Rect]::new()
if (-not [AgentPulseE2EWindowCapture]::GetWindowRect($window.MainWindowHandle, [ref]$rect)) {
    throw 'Could not read the AgentPulse E2E window bounds.'
}
$width = $rect.Right - $rect.Left
$height = $rect.Bottom - $rect.Top
if ($width -lt 320 -or $height -lt 240) {
    throw "The AgentPulse E2E window is too small to review ($width x $height)."
}

$bitmap = [Drawing.Bitmap]::new($width, $height)
$graphics = [Drawing.Graphics]::FromImage($bitmap)
try {
    $graphics.CopyFromScreen(
        $rect.Left,
        $rect.Top,
        0,
        0,
        [Drawing.Size]::new($width, $height),
        [Drawing.CopyPixelOperation]::SourceCopy
    )
    $path = Join-Path $output ($Name + '.png')
    $bitmap.Save($path, [Drawing.Imaging.ImageFormat]::Png)
    [pscustomobject]@{
        Path = $path
        Width = $width
        Height = $height
        ProcessId = $window.Id
    } | ConvertTo-Json -Compress
}
finally {
    $graphics.Dispose()
    $bitmap.Dispose()
}
