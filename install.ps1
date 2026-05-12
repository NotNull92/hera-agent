$ErrorActionPreference = "Stop"

$repo = "NotNull92/hera-agent"
$installDir = "$env:LOCALAPPDATA\\hera-agent"
$exe = "$installDir\\hera-agent.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$url = "https://github.com/$repo/releases/latest/download/hera-agent-windows-amd64.exe"
Write-Host "Downloading hera-agent for windows/amd64..."
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$installDir;$userPath", "User")
    $env:Path = "$installDir;$env:Path"
    Write-Host "Added $installDir to PATH (restart shell to apply)"
}

Write-Host "Installed hera-agent to $exe"
& $exe version
