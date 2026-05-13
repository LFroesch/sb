$ErrorActionPreference = "Stop"

function Require-Go {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw "Go is required for native Windows installs of sb. Install Go first, then rerun this script."
    }
}

Require-Go

go install github.com/LFroesch/sb@latest
if ($LASTEXITCODE -ne 0) {
    throw "go install github.com/LFroesch/sb@latest failed"
}

go install github.com/LFroesch/sb/cmd/foreman@latest
if ($LASTEXITCODE -ne 0) {
    throw "go install github.com/LFroesch/sb/cmd/foreman@latest failed"
}

Write-Host "Installed sb and sb-foreman via go install."
Write-Host "Ensure your Go bin directory is on PATH, then run: sb"
