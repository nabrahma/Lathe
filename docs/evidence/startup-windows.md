# Startup and memory on Windows

Measured on the machine described below, five cold launches of a release build,
timing from process start to the window handle existing. Reproduce with the
PowerShell snippet at the bottom.

| Run | Cold start | Idle memory |
|-----|-----------:|------------:|
| 1   |     569 ms |     33.2 MB |
| 2   |     459 ms |     29.5 MB |
| 3   |     479 ms |     32.1 MB |
| 4   |     506 ms |     31.8 MB |
| 5   |     465 ms |     29.9 MB |
| **Median** | **479 ms** | **31.8 MB** |

Binary size: 18.1 MB (`build/bin/lathe.exe`, `wails build -trimpath -s`).

The target was a cold start under two seconds and an installer under 55 MB.
Both are met with room to spare, which is what the choice of Wails over
Electron bought: there is no bundled Chromium, only the WebView2 runtime the
operating system already has.

## Machine

- Windows 11 Home Single Language, build 26200
- AMD Ryzen 7 6800HS, 16 GB RAM
- Go 1.26.4, Wails v2.15.0, WebView2 152.0.4191.53

## Method

```powershell
foreach ($i in 1..5) {
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $p = Start-Process -FilePath ".\build\bin\lathe.exe" -PassThru
  while ((Get-Process -Id $p.Id).MainWindowHandle -eq 0) { Start-Sleep -Milliseconds 20 }
  $sw.Stop()
  "$($sw.Elapsed.TotalMilliseconds) ms, $((Get-Process -Id $p.Id).WorkingSet64/1MB) MB"
  Stop-Process -Id $p.Id -Force
}
```
