[CmdletBinding()]
param(
    [switch]$Race,
    [switch]$Vulnerability
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $goFiles = Get-ChildItem -LiteralPath $root -Recurse -File -Filter '*.go' |
        Where-Object { $_.FullName -notlike "$root\data\*" } |
        ForEach-Object { $_.FullName }
    $unformatted = @(& gofmt -l $goFiles)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt执行失败' }
    if ($unformatted.Count -gt 0) {
        throw "以下Go文件未格式化:`n$($unformatted -join "`n")"
    }

    & go mod tidy -diff
    if ($LASTEXITCODE -ne 0) { throw 'go.mod/go.sum需要整理' }
    & go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go单元测试失败' }
    $coverageFile = Join-Path ([IO.Path]::GetTempPath()) ("ctf-agent-coverage-" + [Guid]::NewGuid().ToString('N'))
    try {
        & go test -count=1 -coverprofile $coverageFile ./internal/app
        if ($LASTEXITCODE -ne 0) { throw 'Go覆盖率测试失败' }
        $coverageSummary = (& go tool cover -func $coverageFile | Select-Object -Last 1)
        if ($LASTEXITCODE -ne 0 -or $coverageSummary -notmatch '([0-9]+(?:\.[0-9]+)?)%') {
            throw '无法读取Go覆盖率'
        }
        $coveragePercent = [double]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
        if ($coveragePercent -lt 70.0) { throw "internal/app覆盖率$coveragePercent%，低于70%" }
        Write-Host "internal/app coverage: $coveragePercent%"
    } finally {
        Remove-Item -LiteralPath $coverageFile -Force -ErrorAction SilentlyContinue
    }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet失败' }

    $buildDir = Join-Path ([IO.Path]::GetTempPath()) ("ctf-agent-build-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $buildDir | Out-Null
    try {
        & go build -o (Join-Path $buildDir 'go-server') ./cmd/go-server
        if ($LASTEXITCODE -ne 0) { throw 'Go服务构建失败' }
        & go build -o (Join-Path $buildDir 'fake-provider') ./cmd/fake-provider
        if ($LASTEXITCODE -ne 0) { throw '假Provider构建失败' }
    } finally {
        Remove-Item -LiteralPath $buildDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    & python -m py_compile runtime/opencode/bridge.py
    if ($LASTEXITCODE -ne 0) { throw 'Python语法检查失败' }
    & python -m unittest discover -s runtime/opencode -p 'test_*.py'
    if ($LASTEXITCODE -ne 0) { throw 'Python单元测试失败' }
    & python scripts/check-python-coverage.py
    if ($LASTEXITCODE -ne 0) { throw 'Python桥接核心覆盖率未达标' }
    Get-ChildItem web/static -Filter '*.js' | ForEach-Object {
        & node --check $_.FullName
        if ($LASTEXITCODE -ne 0) { throw "JavaScript语法检查失败:$($_.Name)" }
    }
    & ./scripts/check-markdown-links.ps1
    if ($LASTEXITCODE -ne 0) { throw 'Markdown链接检查失败' }

    if ($Race) {
        & go test -count=1 -race ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go竞态检查失败' }
    }
    if ($Vulnerability) {
        & go run golang.org/x/vuln/cmd/govulncheck@latest ./...
        if ($LASTEXITCODE -ne 0) { throw 'govulncheck失败' }
    }
    Write-Host '全部确定性检查通过。'
} finally {
    Pop-Location
}
