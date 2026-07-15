[CmdletBinding()]
param(
    [string]$Root = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = 'Stop'
$rootPath = [IO.Path]::GetFullPath($Root)
$broken = [Collections.Generic.List[string]]::new()
$checked = 0
$pattern = [regex]'(?<!!)\[[^\]]*\]\((?<target>[^)]+)\)'

Get-ChildItem -LiteralPath $rootPath -Recurse -File -Filter '*.md' |
    Where-Object {
        $_.FullName -notlike "$rootPath\.git\*" -and
        $_.FullName -notlike "$rootPath\data\*"
    } |
    ForEach-Object {
        $document = $_
        $content = [IO.File]::ReadAllText($document.FullName)
        foreach ($match in $pattern.Matches($content)) {
            $target = $match.Groups['target'].Value.Trim()
            if ($target.StartsWith('<') -and $target.EndsWith('>')) {
                $target = $target.Substring(1, $target.Length - 2)
            }
            if ($target -match '^(?:https?://|mailto:|#)' -or $target -eq '') {
                continue
            }
            if ($target -match '[''"\[\]$\r\n]' -or $target -match '\s') {
                continue
            }
            $pathPart = ($target -split '#', 2)[0]
            if ($pathPart -eq '') {
                continue
            }
            $pathPart = [Uri]::UnescapeDataString($pathPart)
            $candidate = [IO.Path]::GetFullPath((Join-Path $document.DirectoryName $pathPart))
            $checked++
            if (-not (Test-Path -LiteralPath $candidate)) {
                $relativeDocument = [IO.Path]::GetRelativePath($rootPath, $document.FullName)
                $broken.Add("${relativeDocument}: $target")
            }
        }
    }

if ($broken.Count -gt 0) {
    $broken | Sort-Object | ForEach-Object { Write-Error "broken markdown link: $_" -ErrorAction Continue }
    Write-Host "Markdown links failed: $($broken.Count) broken of $checked local links."
    exit 1
}

Write-Host "Markdown links passed: $checked local links checked."
