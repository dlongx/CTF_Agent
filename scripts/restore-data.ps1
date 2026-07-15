[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Archive,
    [string]$DataDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'data')
)

$ErrorActionPreference = 'Stop'
$archivePath = [IO.Path]::GetFullPath($Archive)
if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf)) {
    throw "备份文件不存在:$archivePath"
}

$target = [IO.Path]::GetFullPath($DataDir)
$parent = Split-Path -Parent $target
New-Item -ItemType Directory -Path $parent -Force | Out-Null
$staging = Join-Path $parent ('.ctf-agent-restore-' + [Guid]::NewGuid().ToString('N'))
$previous = "$target.previous-" + [DateTimeOffset]::Now.ToString('yyyyMMdd-HHmmss')
New-Item -ItemType Directory -Path $staging | Out-Null
try {
    Expand-Archive -LiteralPath $archivePath -DestinationPath $staging
    $payload = Join-Path $staging 'data'
    $manifestPath = Join-Path $staging 'manifest.json'
    if (-not (Test-Path -LiteralPath $payload -PathType Container) -or -not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw '备份缺少data目录或manifest.json'
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($manifest.format_version -ne 1) { throw "不支持的备份格式:$($manifest.format_version)" }
    foreach ($entry in $manifest.files) {
        $candidate = [IO.Path]::GetFullPath((Join-Path $payload $entry.path))
        if (-not $candidate.StartsWith(([IO.Path]::GetFullPath($payload) + [IO.Path]::DirectorySeparatorChar), [StringComparison]::OrdinalIgnoreCase)) {
            throw "备份包含越界路径:$($entry.path)"
        }
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { throw "备份文件缺失:$($entry.path)" }
        $hash = (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($hash -ne $entry.sha256 -or (Get-Item -LiteralPath $candidate).Length -ne $entry.bytes) {
            throw "备份校验失败:$($entry.path)"
        }
    }

    if (Test-Path -LiteralPath $target) {
        Move-Item -LiteralPath $target -Destination $previous
    }
    try {
        Move-Item -LiteralPath $payload -Destination $target
    } catch {
        if ((Test-Path -LiteralPath $previous) -and -not (Test-Path -LiteralPath $target)) {
            Move-Item -LiteralPath $previous -Destination $target
        }
        throw
    }
    Write-Host "恢复完成:$target"
    if (Test-Path -LiteralPath $previous) { Write-Host "原数据保留在:$previous" }
} finally {
    Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
}
