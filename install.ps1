$ErrorActionPreference = "Stop"

$repo = "NotNull92/hera-agent"
$installDir = "$env:LOCALAPPDATA\hera-agent"
$exe = "$installDir\hera-agent.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$url = "https://github.com/$repo/releases/latest/download/hera-agent-windows-amd64.exe"
Write-Host "Downloading hera-agent for windows/amd64..."
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

# Normalize User PATH: drop any existing entry pointing to the install directory
# (including stale double-backslash variants from older installs), then prepend the canonical path once.
$installDirNorm = [System.IO.Path]::GetFullPath($installDir).TrimEnd('\')
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = if ($userPath) { $userPath -split ';' } else { @() }
$filtered = $entries | Where-Object {
    if (-not $_) { return $false }
    try { return ([System.IO.Path]::GetFullPath($_).TrimEnd('\')) -ne $installDirNorm }
    catch { return $true }
}
$newPath = ((@($installDir) + $filtered) | Where-Object { $_ }) -join ';'

if ($newPath -ne $userPath) {
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "PATH updated."
}

# Update current session
$sessionEntries = if ($env:Path) { $env:Path -split ';' } else { @() }
$sessionFiltered = $sessionEntries | Where-Object {
    if (-not $_) { return $false }
    try { return ([System.IO.Path]::GetFullPath($_).TrimEnd('\')) -ne $installDirNorm }
    catch { return $true }
}
$env:Path = ((@($installDir) + $sessionFiltered) | Where-Object { $_ }) -join ';'

Write-Host "Installed hera-agent to $exe"
Write-Host ""
Write-Host "If 'hera-agent' is not recognized in an already-open IDE or terminal,"
Write-Host "fully close and reopen that application (not just the terminal tab)"
Write-Host "so it picks up the updated PATH."
Write-Host ""
& $exe version
