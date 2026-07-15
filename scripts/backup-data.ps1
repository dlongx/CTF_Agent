[CmdletBinding()]
param(
    [string]$DataDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'data'),
    [string]$OutputDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'backups')
)

$ErrorActionPreference = 'Stop'
$source = [IO.Path]::GetFullPath($DataDir)
if (-not (Test-Path -LiteralPath $source -PathType Container)) {
    throw "数据目录不存在:$source"
}

$output = [IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $output -Force | Out-Null
$stamp = [DateTimeOffset]::Now.ToString('yyyyMMdd-HHmmss')
$staging = Join-Path ([IO.Path]::GetTempPath()) ("ctf-agent-backup-$stamp-" + [Guid]::NewGuid().ToString('N'))
$archive = Join-Path $output "ctf-agent-data-$stamp.zip"
New-Item -ItemType Directory -Path $staging | Out-Null
try {
    $payload = Join-Path $staging 'data'
    Copy-Item -LiteralPath $source -Destination $payload -Recurse
    $files = Get-ChildItem -LiteralPath $payload -Recurse -File | Sort-Object FullName
    $manifest = foreach ($file in $files) {
        [pscustomobject]@{
            path = [IO.Path]::GetRelativePath($payload, $file.FullName).Replace('\', '/')
            bytes = $file.Length
            sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    [pscustomobject]@{
        format_version = 1
        created_at = [DateTimeOffset]::UtcNow.ToString('o')
        files = @($manifest)
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $staging 'manifest.json') -Encoding utf8NoBOM
    Compress-Archive -LiteralPath (Join-Path $staging 'data'), (Join-Path $staging 'manifest.json') -DestinationPath $archive -CompressionLevel Optimal
    Write-Host "备份完成:$archive"
} finally {
    Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
}
