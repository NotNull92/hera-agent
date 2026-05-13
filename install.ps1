$ErrorActionPreference = "Stop"

$repo = "NotNull92/hera-agent"

# Install into WindowsApps — Windows 10+ keeps this on the default user PATH,
# so new terminals (and IDEs launched from a fresh shell) pick up hera-agent
# without us touching the PATH registry at all.
$installDir = "$env:LOCALAPPDATA\Microsoft\WindowsApps"
$exe = "$installDir\hera-agent.exe"

# Migrate from legacy install location ($LOCALAPPDATA\hera-agent) if present.
$legacyDir = "$env:LOCALAPPDATA\hera-agent"
$legacyExe = "$legacyDir\hera-agent.exe"
if (Test-Path $legacyExe) {
    try {
        Remove-Item -Path $legacyExe -Force -ErrorAction Stop
        Write-Host "Removed legacy binary at $legacyExe"
    } catch {
        Write-Host "Warning: could not remove legacy binary: $_"
    }
}
if ((Test-Path $legacyDir) -and (-not (Get-ChildItem -Path $legacyDir -Force -ErrorAction SilentlyContinue))) {
    try { Remove-Item -Path $legacyDir -Force -ErrorAction Stop } catch { }
}

# Clean any legacy PATH entries pointing at the old install dir
# (single OR double backslash variants from earlier installs).
$legacyNorm = [System.IO.Path]::GetFullPath($legacyDir).TrimEnd('\')
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = if ($userPath) { $userPath -split ';' } else { @() }
$filtered = $entries | Where-Object {
    if (-not $_) { return $false }
    try { return ([System.IO.Path]::GetFullPath($_).TrimEnd('\')) -ne $legacyNorm }
    catch { return $true }
}
$newPath = $filtered -join ';'
if ($newPath -ne $userPath) {
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Cleaned legacy PATH entries"
}

# Download the binary into the canonical WindowsApps location.
$url = "https://github.com/$repo/releases/latest/download/hera-agent-windows-amd64.exe"
Write-Host "Downloading hera-agent for windows/amd64..."
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

Write-Host ""
Write-Host "Installed hera-agent to $exe"
Write-Host ""
Write-Host "Any NEW terminal or IDE will see 'hera-agent' immediately"
Write-Host "(WindowsApps is on the default user PATH)."
Write-Host ""
Write-Host "If an ALREADY-OPEN terminal still doesn't see it, refresh its"
Write-Host "PATH in that session with:"
Write-Host '    $env:Path = [Environment]::GetEnvironmentVariable("Path","User") + ";" + [Environment]::GetEnvironmentVariable("Path","Machine")'
Write-Host ""
Write-Host "Next, get your AI agent to use it:"
Write-Host "  - Discover: ask Claude Code CLI or Codex in any terminal:"
Write-Host '      "Check whether the hera-agent CLI tool is installed and explore its capabilities."'
Write-Host "  - Lock in (recommended): add to your project's CLAUDE.md / AGENTS.md:"
Write-Host '      "For any Unity work, always use hera-agent."'
Write-Host ""
& $exe version
