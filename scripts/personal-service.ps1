param(
    [ValidateSet('Start', 'Stop', 'Status')][string]$Action = 'Start',
    [switch]$NoBrowser,
    [switch]$Rebuild
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$state = Join-Path $repo '.local-service'
$compose = Join-Path $repo 'docker-compose.personal.yml'
$envFile = Join-Path $state 'runtime.env'
$url = 'http://127.0.0.1:5174'
$lock = $null
$restoreOnBuildFailure = $false

function Docker {
    & docker.exe @args
    if ($LASTEXITCODE -ne 0) { throw "Docker command failed (exit $LASTEXITCODE)." }
}
function Compose {
    Docker compose --env-file $envFile -f $compose @args
}
function Wait-Database([string]$container) {
    $deadline = (Get-Date).AddMinutes(3)
    do {
        & docker.exe exec $container pg_isready -U postgres -d WeKnora *> $null
        if ($LASTEXITCODE -eq 0) { return }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)
    throw "Database is not ready: $container"
}
function Backup-Database([string]$container, [string]$name) {
    Docker exec $container pg_dump -U postgres -d WeKnora -Fc -f /tmp/personal-backup.dump
    Docker exec $container pg_restore --list /tmp/personal-backup.dump | Out-Null
    Docker cp "${container}:/tmp/personal-backup.dump" (Join-Path $state "backups/$name.dump")
    if ((Get-Item (Join-Path $state "backups/$name.dump")).Length -lt 1024) { throw 'Database backup is incomplete.' }
}

try {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Install Docker Desktop first.' }
    & docker.exe info --format '{{.ServerVersion}}' *> $null
    if ($LASTEXITCODE -ne 0) {
        if ($Action -ne 'Start') { throw 'Docker Desktop is not running.' }
        $desktop = Join-Path $env:ProgramFiles 'Docker/Docker/Docker Desktop.exe'
        if (-not (Test-Path $desktop)) { throw 'Docker Desktop executable was not found.' }
        Write-Host 'Starting Docker Desktop...'
        Start-Process -FilePath $desktop -WindowStyle Hidden
        $deadline = (Get-Date).AddMinutes(4)
        do {
            Start-Sleep -Seconds 3
            & docker.exe info --format '{{.ServerVersion}}' *> $null
            if ($LASTEXITCODE -eq 0) { break }
        } while ((Get-Date) -lt $deadline)
        if ($LASTEXITCODE -ne 0) { throw 'Docker Desktop did not become ready within 4 minutes.' }
    }
    if ($Action -eq 'Status') {
        if (Test-Path $envFile) { Compose ps -a }
        else { Write-Host 'Personal service has not been initialized.' }
        exit 0
    }
    New-Item -ItemType Directory -Force $state | Out-Null
    try { $lock = [IO.File]::Open((Join-Path $state 'launcher.lock'), 'OpenOrCreate', 'ReadWrite', 'None') }
    catch { throw 'Another launcher is running. Wait for it to finish.' }
    if ($Action -ne 'Start') {
        if (-not (Test-Path $envFile)) { Write-Host 'Personal service has not been initialized.'; exit 0 }
        if ($Action -eq 'Stop') { Compose stop }
        Compose ps -a
        exit 0
    }
    foreach ($dir in @('bin', 'backups', 'files', 'dist', 'logs')) {
        New-Item -ItemType Directory -Force (Join-Path $state $dir) | Out-Null
    }
    if (-not (Test-Path $envFile)) {
        $workspace = Split-Path -Parent (Split-Path -Parent $repo)
        $seed = Join-Path $workspace 'WeKnora-topic3/.env'
        if (-not (Test-Path $seed)) { throw "Existing configuration is missing: $seed" }
        Copy-Item -LiteralPath $seed -Destination $envFile
    }
    $initialized = Join-Path $state 'initialized'
    if (-not (Test-Path $initialized)) {
        Write-Host 'Backing up existing development data; initializing an independent database...'
        $sourceWasRunning = (Docker inspect --format '{{.State.Running}}' WeKnora-postgres-dev).Trim() -eq 'true'
        try {
            Docker start WeKnora-postgres-dev | Out-Null
            Wait-Database 'WeKnora-postgres-dev'
            Backup-Database 'WeKnora-postgres-dev' 'source-development'
        } finally {
            if (-not $sourceWasRunning) { Docker stop WeKnora-postgres-dev | Out-Null }
        }
        Compose up -d --wait --wait-timeout 180 postgres
        $db = (Compose ps -q postgres).Trim()
        # Never restore over a partially initialized or already populated database.
        $count = (Docker exec $db psql -U postgres -d WeKnora -Atc "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'spatial_ref_sys';").Trim()
        if ([int]$count -ne 0) { throw 'Personal database already contains tables. Initialization stopped; data was preserved. Inspect the previous failure before retrying.' }
        Docker cp (Join-Path $state 'backups/source-development.dump') "${db}:/tmp/source.dump"
        Docker exec $db pg_restore -U postgres -d WeKnora --clean --if-exists --exit-on-error --single-transaction /tmp/source.dump
        $sourceVersion = (Docker exec $db psql -U postgres -d WeKnora -Atc 'SELECT version FROM schema_migrations;').Trim()
        if ($sourceVersion -eq '103') {
            Docker cp (Join-Path $PSScriptRoot 'personal-restore-checks.sql') "${db}:/tmp/personal-restore-checks.sql"
            Docker exec $db psql -U postgres -d WeKnora -v ON_ERROR_STOP=1 -f /tmp/personal-restore-checks.sql
        }
        $sourceFiles = Join-Path (Split-Path -Parent (Split-Path -Parent $repo)) 'WeKnora-topic3/.local-data/files'
        if (Test-Path $sourceFiles) {
            Get-ChildItem -LiteralPath $sourceFiles -Force | Copy-Item -Destination (Join-Path $state 'files') -Recurse -Force
        }
        Set-Content -LiteralPath $initialized -Value 'Development database copied; subsequent starts use personal data.'
    }

    $stamp = Join-Path $state 'build-stamp.txt'
    # Hash source files, including uncommitted changes, so a restart cannot silently use an old build.
    Write-Host 'Checking application source...'
    $inputs = @('cmd','internal','migrations','config','frontend/src','frontend/public','frontend/scripts','frontend/patches')
    $files = foreach ($inputPath in $inputs) {
        $full = Join-Path $repo $inputPath
        if (Test-Path $full) { Get-ChildItem -LiteralPath $full -Recurse -File }
    }
    $files += Get-ChildItem -LiteralPath $repo -File | Where-Object { $_.Name -match '^(go\.(mod|sum)|Makefile|VERSION)$' }
    $files += Get-Item -LiteralPath (Join-Path $repo 'scripts/personal-build.sh')
    $files += Get-Item -LiteralPath (Join-Path $repo 'scripts/personal-frontend-build.sh')
    $files += Get-ChildItem -LiteralPath (Join-Path $repo 'frontend') -File
    $fileSha = [Security.Cryptography.SHA256]::Create()
    try {
        $hashes = $files | Sort-Object FullName | ForEach-Object {
            $stream = [IO.File]::OpenRead($_.FullName)
            try { $digest = [BitConverter]::ToString($fileSha.ComputeHash($stream)).Replace('-', '') }
            finally { $stream.Dispose() }
            $_.FullName.Substring($repo.Length) + ':' + $digest
        }
    } finally { $fileSha.Dispose() }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $fingerprint = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes(($hashes -join "`n")))) }
    finally { $sha.Dispose() }
    $needsBuild = $Rebuild -or -not (Test-Path $stamp) -or -not (Test-Path (Join-Path $state 'bin/WeKnora')) -or -not (Test-Path (Join-Path $state 'dist/index.html'))
    if (-not $needsBuild) { $needsBuild = (Get-Content -LiteralPath $stamp -Raw).Trim() -ne $fingerprint }
    if ($needsBuild) {
        Write-Host 'Building current backend and web application. First build can take several minutes...'
        $restoreOnBuildFailure = (Test-Path (Join-Path $state 'bin/WeKnora')) -and (Test-Path (Join-Path $state 'dist/index.html'))
        # Stop only this project's application before replacing executable/static assets.
        Compose stop frontend app
        Compose up -d --wait --wait-timeout 180 postgres
        $db = (Compose ps -q postgres).Trim()
        Backup-Database $db ('before-build-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
        $buildCommit = 'unknown'
        if (Get-Command git -ErrorAction SilentlyContinue) {
            $revision = & git -c "safe.directory=$repo" -C $repo rev-parse HEAD 2>$null
            if ($LASTEXITCODE -eq 0) {
                $buildCommit = $revision.Trim()
                $changes = & git -c "safe.directory=$repo" -C $repo status --porcelain 2>$null
                if ($changes) { $buildCommit += '-dirty' }
            }
        }
        Docker run --rm --entrypoint sh -e "WEKNORA_BUILD_COMMIT=$buildCommit" -v "${repo}:/app" -v weknora-go-mod:/go/pkg/mod -v weknora-go-build:/root/.cache/go-build -w /app weknora-nogit-go:1.26 scripts/personal-build.sh
        Docker run --rm --entrypoint sh -v "${repo}:/app" -v weknora-personal-npm:/root/.npm -w /app node:22-alpine scripts/personal-frontend-build.sh
        Set-Content -LiteralPath $stamp -Value $fingerprint
    }
    Write-Host 'Starting PostgreSQL, Redis, DocReader, backend and web application...'
    Compose up -d --wait --wait-timeout 300
    $restoreOnBuildFailure = $false
    $ready = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:18080/health/ready' -TimeoutSec 10
    $page = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 10
    if ($ready.StatusCode -ne 200 -or $page.StatusCode -ne 200) { throw 'Service readiness check failed.' }
    Compose ps
    $lastError = Join-Path $state 'last-error.txt'
    if (Test-Path $lastError) { Remove-Item -LiteralPath $lastError }
    Write-Host "WeKnora is ready: $url" -ForegroundColor Green
    if (-not $NoBrowser) { Start-Process $url }
} catch {
    $startupError = $_.Exception.Message
    if ($restoreOnBuildFailure) {
        try {
            Compose up -d --wait --wait-timeout 180
            Write-Host 'The existing service has been started after the build failure.'
        } catch { Write-Host 'Automatic service recovery failed; inspect the container status.' -ForegroundColor Yellow }
    }
    Write-Host ("Startup failed: " + $startupError) -ForegroundColor Red
    Write-Host 'Existing data is retained. See .local-service/last-error.txt and run the status launcher.'
    if (Test-Path $state) { $startupError | Set-Content -LiteralPath (Join-Path $state 'last-error.txt') }
    exit 1
} finally {
    if ($lock) { $lock.Dispose() }
}
