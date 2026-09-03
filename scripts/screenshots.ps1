# Captures the screenshots used in the README.
#
# Drives the real application rather than assembling mockups, so a screenshot
# cannot drift from what the app actually looks like, and every number in one
# is a number the app produced.
#
#   go run ./scripts/demofiles
#   make build
#   powershell -ExecutionPolicy Bypass -File scripts/screenshots.ps1
#
# The capture process must declare itself DPI aware before it touches a window.
# Without that, GetWindowRect answers in logical coordinates while the screen
# grab works in physical pixels, so the captured region is the wrong size and
# every click lands somewhere other than where it was aimed.

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Shot {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int ht, bool repaint);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint x, uint y, uint d, int e);
  public struct RECT { public int Left, Top, Right, Bottom; }
}
"@

[Shot]::SetProcessDPIAware() | Out-Null

$root = Split-Path -Parent $PSScriptRoot
$out  = Join-Path $root "docs\screenshots"
$exe  = Join-Path $root "build\bin\lathe.exe"
$demo = Join-Path $env:USERPROFILE "Documents\Lathe Demo"

if (-not (Test-Path $exe))  { throw "Build the app first: make build" }
if (-not (Test-Path $demo)) { throw "Generate the demo files first: go run ./scripts/demofiles" }
New-Item -ItemType Directory -Force -Path $out | Out-Null

# The window size every click coordinate below was measured against.
$windowWidth  = 1900
$windowHeight = 1150

$script:window = $null
$script:handle = 0

function Open-App([string] $file) {
  Get-Process lathe -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Milliseconds 500

  # Start from first-run state, so a remembered window position from an
  # earlier capture does not move the layout under the click coordinates.
  Remove-Item (Join-Path $env:APPDATA "Lathe\settings.json") -Force -ErrorAction SilentlyContinue

  $p = if ($file) {
    Start-Process -FilePath $exe -ArgumentList "`"$file`"" -PassThru
  } else {
    Start-Process -FilePath $exe -PassThru
  }
  Start-Sleep -Seconds 5

  $script:handle = (Get-Process -Id $p.Id).MainWindowHandle
  [Shot]::MoveWindow($script:handle, 60, 20, $windowWidth, $windowHeight, $true) | Out-Null
  [Shot]::SetForegroundWindow($script:handle) | Out-Null
  Start-Sleep -Seconds 2

  $script:window = New-Object Shot+RECT
  [Shot]::GetWindowRect($script:handle, [ref]$script:window) | Out-Null

  $actual = $script:window.Right - $script:window.Left
  if ($actual -ne $windowWidth) {
    throw "window came out $actual px wide, expected $windowWidth; click coordinates would be wrong"
  }
}

function Save-Shot([string] $name) {
  $r = New-Object Shot+RECT
  [Shot]::GetWindowRect($script:handle, [ref]$r) | Out-Null

  # Park the pointer on empty chrome first. Left where it clicked it hovers a
  # card, and left over the desktop it raises a Windows tooltip; both end up in
  # the picture.
  [System.Windows.Forms.Cursor]::Position =
    New-Object System.Drawing.Point (($r.Right - 40), ($r.Top + 200))
  Start-Sleep -Milliseconds 700

  $bmp = New-Object System.Drawing.Bitmap ($r.Right - $r.Left), ($r.Bottom - $r.Top)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.Left, $r.Top, 0, 0, $bmp.Size)
  $bmp.Save((Join-Path $out $name), [System.Drawing.Imaging.ImageFormat]::Png)
  $g.Dispose(); $bmp.Dispose()
  Write-Host "  $name"
}

function Invoke-Click([int] $x, [int] $y, [int] $settle = 1200) {
  $point = New-Object System.Drawing.Point (($script:window.Left + $x), ($script:window.Top + $y))
  [System.Windows.Forms.Cursor]::Position = $point
  Start-Sleep -Milliseconds 300
  [Shot]::mouse_event(0x0002, 0, 0, 0, 0)
  [Shot]::mouse_event(0x0004, 0, 0, 0, 0)
  Start-Sleep -Milliseconds $settle
}

# Results from an earlier capture would otherwise collide, and the run would
# show "name (2).pdf" rather than the name a first-time user sees.
Get-ChildItem $demo -Filter "*-compressed*" -ErrorAction SilentlyContinue | Remove-Item -Force

Write-Host "capturing:"

# The whole task grid, with nothing selected.
Open-App $null
Save-Shot "01-home.png"

# Launched with a scan, so the grid opens filtered to what a PDF can become.
# This is the same state a drag onto the window produces.
Open-App "$demo\Fee Receipt Scan.pdf"
Save-Shot "02-filtered.png"

# Compress PDF, which arrives with the file already attached.
Invoke-Click 254 807
Save-Shot "03-task.png"

# Run it. The size figures on the result are whatever the app produced.
Invoke-Click 1775 770 6000
Save-Shot "04-result.png"

# Settings, where the tier system is visible.
Open-App $null
Invoke-Click 1812 353
Save-Shot "05-settings.png"

Get-Process lathe -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host "done"
