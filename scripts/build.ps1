param(
    [string]$Output = "build/verba.exe"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

go test ./...
New-Item -ItemType Directory -Force (Split-Path -Parent $Output) | Out-Null
go build -trimpath -ldflags "-s -w" -o $Output ./cmd/verba
Write-Host "Built $Output"
