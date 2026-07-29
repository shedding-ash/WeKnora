param(
    [string]$WslPath = "/home/liusz10/wiki/WeKnora-fork",
    [string]$Branch = "",
    [string]$Remote = "origin",
    [string]$ExpectedSha = "",
    [int]$WaitSeconds = 90,
    [switch]$SkipRemoteWait
)

$ErrorActionPreference = "Stop"

function ConvertTo-WslPath {
    param([Parameter(Mandatory = $true)][string]$WindowsPath)

    $fullPath = [System.IO.Path]::GetFullPath($WindowsPath)
    if ($fullPath -notmatch "^([A-Za-z]):\\(.*)$") {
        throw "Only drive-letter Windows paths are supported: $WindowsPath"
    }

    $drive = $Matches[1].ToLowerInvariant()
    $rest = $Matches[2] -replace "\\", "/"
    return "/mnt/$drive/$rest"
}

function Invoke-WslGit {
    param([Parameter(Mandatory = $true)][string]$Script)

    $cleanPath = "/home/liusz10/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    $escapedWslPath = $WslPath -replace "'", "'\''"
    $command = @"
set -euo pipefail
export PATH="$cleanPath"
cd '$escapedWslPath'
$Script
"@

    $tempScript = New-TemporaryFile
    try {
        $command = $command -replace "`r`n", "`n"
        [System.IO.File]::WriteAllText($tempScript.FullName, $command, [System.Text.Encoding]::ASCII)
        $wslScriptPath = ConvertTo-WslPath $tempScript.FullName

        & wsl.exe bash $wslScriptPath
        if ($LASTEXITCODE -ne 0) {
            throw "WSL command failed with exit code $LASTEXITCODE"
        }
    } finally {
        Remove-Item -LiteralPath $tempScript.FullName -Force -ErrorAction SilentlyContinue
    }
}

if (-not $Branch) {
    $Branch = (git branch --show-current).Trim()
}

if (-not $Branch) {
    throw "Cannot determine branch. Pass -Branch explicitly."
}

if (-not $SkipRemoteWait -and $ExpectedSha) {
    $deadline = (Get-Date).AddSeconds($WaitSeconds)
    do {
        $remoteSha = (& git ls-remote $Remote "refs/heads/$Branch" | ForEach-Object { ($_ -split "\s+")[0] }).Trim()
        if ($remoteSha -eq $ExpectedSha) {
            break
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)

    if ($remoteSha -ne $ExpectedSha) {
        Write-Warning "Remote $Remote/$Branch did not reach $ExpectedSha within $WaitSeconds seconds. Skipping WSL sync."
        exit 0
    }
}

Write-Host "[sync-wsl] Updating $WslPath from $Remote/$Branch"

Invoke-WslGit @"
if [ -n "`$(git status --porcelain)" ]; then
  echo "[sync-wsl] WSL worktree has local changes; refusing to pull." >&2
  git status --short >&2
  exit 2
fi

current_branch="`$(git branch --show-current)"
if [ "`$current_branch" != "$Branch" ]; then
  git checkout "$Branch"
fi

git fetch "$Remote" "$Branch"
git pull --ff-only "$Remote" "$Branch"
"@

Write-Host "[sync-wsl] Done."
