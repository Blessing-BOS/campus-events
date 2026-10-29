param([string]$BaseUrl = 'http://127.0.0.1:8080')

$ErrorActionPreference = 'Stop'
$BaseUrl = $BaseUrl.TrimEnd('/')

function Assert-HttpStatus([string]$Path, [int]$Expected) {
    $taskStatus = 0
    try {
        $taskResponse = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$Path" -TimeoutSec 10
        $taskStatus = [int]$taskResponse.StatusCode
    }
    catch {
        if ($null -eq $_.Exception.Response) { throw }
        $taskStatus = [int]$_.Exception.Response.StatusCode
    }
    if ($taskStatus -ne $Expected) { throw "GET $Path returned $taskStatus, expected $Expected" }
    Write-Output "PASS GET $Path => $Expected"
}

$taskHealth = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 10
if ($taskHealth.status -ne 'ok') { throw 'Health check failed.' }
Write-Output "PASS /healthz (data_source=$($taskHealth.data_source))"
Assert-HttpStatus '/readyz' 200
$taskPage = Invoke-RestMethod -Uri "$BaseUrl/api/v1/events?limit=1" -TimeoutSec 10
if (@($taskPage.items).Count -ne 1) { throw 'Expected one seeded event on first page.' }
if (-not $taskPage.has_more -or $null -eq $taskPage.next_before_id) { throw 'Expected a next page with the stage 1 demo dataset.' }
$taskFirstID = $taskPage.items[0].id
Write-Output 'PASS first page and next-page cursor'
Assert-HttpStatus "/api/v1/events/$taskFirstID" 200
$taskNextPage = Invoke-RestMethod -Uri "$BaseUrl/api/v1/events?limit=1&before_id=$($taskPage.next_before_id)" -TimeoutSec 10
if (@($taskNextPage.items).Count -ne 1 -or $taskNextPage.items[0].id -ge $taskFirstID) { throw 'Exclusive cursor failed.' }
Write-Output 'PASS next page excludes previous page'
Assert-HttpStatus '/api/v1/events?limit=0' 400
Assert-HttpStatus '/api/v1/events/0' 400
Assert-HttpStatus '/api/v1/events/2147483647' 404
Assert-HttpStatus '/not-a-route' 404
if ($taskHealth.data_source -eq 'demo') {
    Assert-HttpStatus '/api/v1/events/3' 404
}
Write-Output 'All stage 1 HTTP smoke checks passed.'
