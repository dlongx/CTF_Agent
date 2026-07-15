[CmdletBinding()]
param(
    [string]$BaseUrl = 'http://127.0.0.1:18000',
    [string]$DockerImage = 'ctf-agent-misc:latest',
    [int]$TimeoutSeconds = 300
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$work = Join-Path ([IO.Path]::GetTempPath()) ('ctf-agent-integration-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null
$serverBinary = Join-Path $work ($(if ($IsWindows) { 'go-server.exe' } else { 'go-server' }))
$providerBinary = Join-Path $work ($(if ($IsWindows) { 'fake-provider.exe' } else { 'fake-provider' }))
$serverLog = Join-Path $work 'server.log'
$providerLog = Join-Path $work 'provider.log'
$attachment = Join-Path $work 'challenge.txt'
[IO.File]::WriteAllText($attachment, 'The flag is flag{ctf_agent_smoke_ok}.', [Text.UTF8Encoding]::new($false))
$originalEnvironment = @{}
$smokePassed = $false

Push-Location $root
try {
    go build -o $serverBinary ./cmd/go-server
    go build -o $providerBinary ./cmd/fake-provider

    $startProvider = @{
        FilePath = $providerBinary
        ArgumentList = @('-addr', '127.0.0.1:8317')
        RedirectStandardOutput = $providerLog
        RedirectStandardError = (Join-Path $work 'provider.err.log')
        PassThru = $true
    }
    if ($IsWindows) { $startProvider.WindowStyle = 'Hidden' }
    $provider = Start-Process @startProvider

    $environmentOverrides = [ordered]@{
        CTF_AGENT_GO_ADDR = ([Uri]$BaseUrl).Authority
        CTF_AGENT_DATA_DIR = Join-Path $work 'data'
        CTF_AGENT_DOCKER_IMAGE = $DockerImage
        CTF_AGENT_IMAGE_MISC = $DockerImage
        CTF_AGENT_MAX_CONTAINERS = '1'
        CTF_AGENT_TASK_TIMEOUT = '5m'
        CTF_AGENT_AUTO_CONTINUE_ROUNDS = '1'
        CTF_AGENT_OPENCODE_RUN_TIMEOUT = '3m'
        CTF_AGENT_OPENCODE_IDLE_TIMEOUT = '1m'
        CTF_AGENT_CONTAINER_RETENTION = '1h'
        CTF_AGENT_AGENT_SCRIPT = Join-Path $root 'runtime/opencode/bridge.py'
        CTF_AGENT_SKILLS_DIR = Join-Path $root 'runtime/opencode/skills'
        OPENCODE_PROVIDER_FORMAT = 'openai-compatible'
        OPENCODE_OPENAI_PROVIDER_ID = 'ctf'
        OPENCODE_OPENAI_PROVIDER_NAME = 'Fake Provider'
        OPENCODE_OPENAI_PROVIDER_NPM = '@ai-sdk/openai-compatible'
        OPENCODE_OPENAI_BASE_URL = 'http://127.0.0.1:8317/v1'
        OPENCODE_OPENAI_API_KEY = 'fake-key'
        OPENCODE_OPENAI_MODEL = 'ctf-smoke'
    }
    foreach ($name in $environmentOverrides.Keys) {
        $originalEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $environmentOverrides[$name], 'Process')
    }

    $startServer = @{
        FilePath = $serverBinary
        RedirectStandardOutput = $serverLog
        RedirectStandardError = (Join-Path $work 'server.err.log')
        PassThru = $true
    }
    if ($IsWindows) { $startServer.WindowStyle = 'Hidden' }
    $server = Start-Process @startServer

    $ready = $false
    for ($attempt = 0; $attempt -lt 100; $attempt++) {
        try {
            $health = Invoke-RestMethod -Uri "$BaseUrl/health" -TimeoutSec 2
            if ($health.status -eq 'ok') { $ready = $true; break }
        } catch {
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $ready) { throw "backend did not become ready; logs: $serverLog" }

    $providerTest = Invoke-RestMethod -Uri "$BaseUrl/api/settings/provider/test" -Method Post -ContentType 'application/json' -Body '{"format":"openai-compatible"}' -TimeoutSec 30
    if (-not $providerTest.ok) { throw "fake provider preflight failed: $($providerTest.error_code)" }

    $task = Invoke-RestMethod -Uri "$BaseUrl/api/tasks" -Method Post -Form @{
        name = 'fake-provider-smoke'
        type = 'misc'
        description = 'Read the attached flag and return it exactly.'
        attachments = Get-Item -LiteralPath $attachment
    } -TimeoutSec 30
    $taskID = $task.id
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        Start-Sleep -Seconds 2
        $task = Invoke-RestMethod -Uri "$BaseUrl/api/tasks/$taskID" -TimeoutSec 10
        if ($task.status -in @('solved', 'failed')) { break }
    } while ([DateTimeOffset]::UtcNow -lt $deadline)

    if ($task.status -ne 'solved' -or $task.flag -ne 'flag{ctf_agent_smoke_ok}') {
        $logs = Invoke-RestMethod -Uri "$BaseUrl/api/tasks/$taskID/logs?tail=30000" -TimeoutSec 10
        throw "integration smoke failed status=$($task.status) flag=$($task.flag)`n$($logs.logs)"
    }
    $writeup = Invoke-WebRequest -Uri "$BaseUrl/api/tasks/$taskID/writeup" -TimeoutSec 10
    if ($writeup.Content -notmatch 'flag\{ctf_agent_smoke_ok\}') {
        throw 'writeup did not contain the expected flag'
    }
    $smokePassed = $true
    Write-Host "Fake-provider integration smoke passed: task_id=$taskID"
} finally {
    if ($taskID) {
        try { Invoke-RestMethod -Uri "$BaseUrl/api/tasks/$taskID/stop" -Method Post -TimeoutSec 5 | Out-Null } catch {}
    }
    if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    if ($provider -and -not $provider.HasExited) { Stop-Process -Id $provider.Id -Force -ErrorAction SilentlyContinue }
    foreach ($name in $originalEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], 'Process')
    }
    Pop-Location
    if ($smokePassed) {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    } else {
        Write-Warning "烟测诊断文件保留在:$work"
    }
}
