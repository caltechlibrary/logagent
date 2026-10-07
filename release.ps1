<#
.SYNOPSIS
Create a draft GitHub release from dist/*.zip. Run `make release` first.
Requires: gh and git. (PowerShell reads codemeta.json itself, so jq is not needed.)
#>
$ErrorActionPreference = "Stop"

foreach ($cmd in "gh", "git") {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        Write-Error "error: $cmd is required"
        exit 1
    }
}

$repoId = Split-Path -Leaf -Path (Get-Location)
$meta = Get-Content -Raw -Path codemeta.json | ConvertFrom-Json
$releaseTag = "v$($meta.version)"
$releaseNotes = "$($meta.releaseNotes)"
if ($releaseTag -notmatch '^v[0-9a-zA-Z._-]+$') {
    Write-Error "error: version contains unexpected characters: ${releaseTag}"
    exit 1
}
$zips = Get-ChildItem -Path dist -Filter *.zip -ErrorAction SilentlyContinue
if (-not $zips) {
    Write-Error "error: no dist/*.zip; run make release first"
    exit 1
}
Write-Output "tag: ${releaseTag}, notes: ${releaseNotes}"

$checksumName = "${repoId}-${releaseTag}-checksums.txt"
$checksumFile = Join-Path dist $checksumName
$zips | ForEach-Object {
    $hash = (Get-FileHash -Path $_.FullName -Algorithm SHA256).Hash.ToLower()
    "$hash  $($_.Name)"
} | Out-File -FilePath $checksumFile -Encoding utf8
Write-Output "Checksums written to ${checksumFile}"

$yesNo = Read-Host -Prompt "Push release to GitHub with gh? (y/N)"
if ($yesNo -eq "y") {
    git commit -am "prep for ${releaseTag}, ${releaseNotes}"
    git push
    gh release create "${releaseTag}" `
        --draft `
        --notes "${releaseNotes}" `
        ($zips | ForEach-Object { $_.FullName }) $checksumFile
    Write-Output "Now go to the repository's releases page and finalize the draft"
}
