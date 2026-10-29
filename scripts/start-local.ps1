param(
    [ValidateRange(1,65535)][int]$Port = 8080,
    [ValidateRange(1,65535)][int]$MySQLPort = 33088,
    [string]$MySQLBin = '',
    [ValidatePattern('^campus_events_[a-z0-9_]+$')][string]$Database = 'campus_events_local'
)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskRun = Join-Path $taskRoot '.run'
New-Item -ItemType Directory -Path $taskRun -Force | Out-Null
if (-not $MySQLBin) {
    $taskCandidates = @(Get-ChildItem -Path "$env:ProgramFiles\MySQL\MySQL Server *\bin\mysqld.exe" -ErrorAction SilentlyContinue | Sort-Object FullName -Descending)
    if (-not $taskCandidates.Count) { throw 'MySQL is not installed. Use Docker Compose or supply -MySQLBin. This script does not install software.' }
    $MySQLBin = $taskCandidates[0].DirectoryName
}
$taskDaemon = Join-Path $MySQLBin 'mysqld.exe'
$taskClient = Join-Path $MySQLBin 'mysql.exe'
$taskAdmin = Join-Path $MySQLBin 'mysqladmin.exe'
foreach ($taskBinary in @($taskDaemon,$taskClient,$taskAdmin)) { if (-not (Test-Path -LiteralPath $taskBinary)) { throw "Missing executable: $taskBinary" } }
$taskSettingsPath = Join-Path $taskRun 'local.settings.json'
if (Test-Path -LiteralPath $taskSettingsPath) {
    $taskSettings = Get-Content -Raw -LiteralPath $taskSettingsPath | ConvertFrom-Json
    if ($taskSettings.mysqlPort -ne $MySQLPort -or $taskSettings.database -ne $Database) { throw 'Existing local settings use a different database/port. Reuse the original options.' }
} else {
    $taskSettings = [pscustomobject]@{ mysqlPort=$MySQLPort; database=$Database; adminPassword=[guid]::NewGuid().ToString('N'); appPassword=[guid]::NewGuid().ToString('N'); jwtSecret=([guid]::NewGuid().ToString('N')+[guid]::NewGuid().ToString('N')); demoPassword='CampusDemo123!'; bootstrapped=$false }
    $taskSettings | ConvertTo-Json | Set-Content -LiteralPath $taskSettingsPath -Encoding UTF8
}
$taskData = Join-Path $taskRun 'local-mysql-data'
if (-not (Test-Path -LiteralPath (Join-Path $taskData 'mysql'))) {
    New-Item -ItemType Directory -Path $taskData -Force | Out-Null
    # Windows PowerShell 5.1 turns redirected native stderr into error records.
    # MySQL writes normal initialization warnings there; check its exit code instead.
    $taskInitArgs = @('--no-defaults','--initialize-insecure',"--basedir=`"$(Split-Path -Parent $MySQLBin)`"","--datadir=`"$taskData`"",'--explicit_defaults_for_timestamp=ON','--console')
    $taskInitProcess = Start-Process -FilePath $taskDaemon -ArgumentList $taskInitArgs -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput (Join-Path $taskRun 'mysql-init.stdout.log') -RedirectStandardError (Join-Path $taskRun 'mysql-init.log')
    if ($taskInitProcess.ExitCode -ne 0) { throw 'MySQL initialization failed. See .run/mysql-init.log.' }
}
function Test-LocalPort([int]$Number) {
    $taskProbe = New-Object Net.Sockets.TcpClient
    try { $taskProbe.Connect('127.0.0.1',$Number); return $true } catch { return $false } finally { $taskProbe.Dispose() }
}
if (Test-LocalPort $Port) { throw "Application port $Port is in use. Choose another -Port." }
$taskStartedDatabase = $false
$taskOldEnv = @{}
foreach ($taskName in @('APP_ADDR','APP_DATA_SOURCE','MYSQL_HOST','MYSQL_PORT','MYSQL_USER','MYSQL_PASSWORD','MYSQL_DATABASE','JWT_SECRET','SEED_PASSWORD','REDIS_ADDR','REDIS_PASSWORD')) { $taskOldEnv[$taskName] = [Environment]::GetEnvironmentVariable($taskName,'Process') }
$taskAdminFile = Join-Path $taskRun 'local-admin.cnf'
Push-Location $taskRoot
try {
    if (-not (Test-LocalPort $MySQLPort)) {
        $taskArgs = @('--no-defaults',"--basedir=`"$(Split-Path -Parent $MySQLBin)`"","--datadir=`"$taskData`"","--port=$MySQLPort",'--bind-address=127.0.0.1','--loose-mysqlx=0','--explicit_defaults_for_timestamp=ON','--console')
        $taskProcess = Start-Process -FilePath $taskDaemon -ArgumentList $taskArgs -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskRun 'local-mysql.stdout.log') -RedirectStandardError (Join-Path $taskRun 'local-mysql.stderr.log')
        $taskStartedDatabase = $true
        $taskDeadline = [DateTime]::UtcNow.AddSeconds(20)
        while (-not (Test-LocalPort $MySQLPort)) {
            if ($taskProcess.HasExited -or [DateTime]::UtcNow -gt $taskDeadline) { throw 'Local MySQL did not start. See .run/local-mysql.stderr.log.' }
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $taskSettings.bootstrapped) {
        $taskSql = "ALTER USER 'root'@'localhost' IDENTIFIED BY '$($taskSettings.adminPassword)'; CREATE DATABASE IF NOT EXISTS $Database CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE USER IF NOT EXISTS 'campus_local'@'127.0.0.1' IDENTIFIED BY '$($taskSettings.appPassword)'; GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,INDEX,REFERENCES ON $Database.* TO 'campus_local'@'127.0.0.1';"
        $taskSql | & $taskClient --no-defaults --host=127.0.0.1 "--port=$MySQLPort" --user=root
        if ($LASTEXITCODE -ne 0) { throw 'Local database bootstrap failed.' }
        $taskSettings.bootstrapped=$true
        $taskSettings | ConvertTo-Json | Set-Content -LiteralPath $taskSettingsPath -Encoding UTF8
    }
    "[client]`nuser=root`npassword=$($taskSettings.adminPassword)`nhost=127.0.0.1`nport=$MySQLPort`nprotocol=tcp" | Set-Content -LiteralPath $taskAdminFile -Encoding ascii
    $env:APP_DATA_SOURCE='mysql';$env:APP_ADDR="127.0.0.1:$Port";$env:MYSQL_HOST='127.0.0.1';$env:MYSQL_PORT=[string]$MySQLPort
    $env:MYSQL_USER='campus_local';$env:MYSQL_PASSWORD=$taskSettings.appPassword;$env:MYSQL_DATABASE=$Database;$env:JWT_SECRET=$taskSettings.jwtSecret;$env:SEED_PASSWORD=$taskSettings.demoPassword
    $env:REDIS_ADDR='';$env:REDIS_PASSWORD=''
    & go run ./cmd/seed
    if ($LASTEXITCODE -ne 0) { throw 'Seed failed.' }
    Write-Output "Open http://127.0.0.1:$Port/"
    Write-Output 'Demo accounts: student_demo / organizer_demo. Password: CampusDemo123!'
    Write-Output 'Press Ctrl+C to stop. Existing Windows MySQL services are not changed.'
    & go run ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "Server exited with code $LASTEXITCODE" }
}
finally {
    if ($taskStartedDatabase -and (Test-Path -LiteralPath $taskAdminFile)) { & $taskAdmin "--defaults-file=$taskAdminFile" shutdown }
    foreach ($taskName in $taskOldEnv.Keys) { [Environment]::SetEnvironmentVariable($taskName,$taskOldEnv[$taskName],'Process') }
    Pop-Location
}
