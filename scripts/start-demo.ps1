param(
    [ValidateRange(1, 65535)]
    [int]$Port = 8080
)

$ErrorActionPreference = 'Stop'
$taskPreviousSource = $env:APP_DATA_SOURCE
$taskPreviousAddress = $env:APP_ADDR
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $env:APP_DATA_SOURCE = 'demo'
    $env:APP_ADDR = "127.0.0.1:$Port"
    & go run ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "Server exited with code $LASTEXITCODE" }
}
finally {
    $env:APP_DATA_SOURCE = $taskPreviousSource
    $env:APP_ADDR = $taskPreviousAddress
    Pop-Location
}
