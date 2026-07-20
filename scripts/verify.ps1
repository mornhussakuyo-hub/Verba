param(
    [string]$BuildDirectory = "build/verify"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$extension = if ($IsWindows -or $env:OS -eq "Windows_NT") { ".exe" } else { "" }
$cliOutput = Join-Path $BuildDirectory ("verba" + $extension)
New-Item -ItemType Directory -Force $BuildDirectory | Out-Null
go build -trimpath -o $cliOutput ./cmd/verba
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$projects = @("examples/hello")
$projects += Get-ChildItem learn -Directory | Sort-Object Name | ForEach-Object { $_.FullName }

go run ./cmd/verba fmt --check examples learn
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

foreach ($project in $projects) {
    $name = Split-Path -Leaf $project
    Write-Host "Verifying $project"
    go run ./cmd/verba check $project
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    go run ./cmd/verba audit --json $project | Out-Null
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    go run ./cmd/verba build -o (Join-Path $BuildDirectory ($name + $extension)) $project
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

Write-Host "Verba verification completed"
