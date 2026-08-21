Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-RepoRoot {
    $root = (& git rev-parse --show-toplevel 2>$null | Out-String).Trim()
    if ([string]::IsNullOrWhiteSpace($root)) {
        throw "Git repository root was not found. Run the script inside a git repository."
    }

    return $root
}

function Test-CommandExists {
    param(
        [Parameter(Mandatory)]
        [string]$Name
    )

    return -not [string]::IsNullOrWhiteSpace((Get-CommandPath -Name $Name))
}

function Get-CommandPath {
    param(
        [Parameter(Mandatory)]
        [string]$Name,
        [string[]]$FallbackPaths = @()
    )

    $command = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $command) {
        return $command.Source
    }

    foreach ($path in $FallbackPaths) {
        if (-not [string]::IsNullOrWhiteSpace($path) -and (Test-Path -LiteralPath $path)) {
            return $path
        }
    }

    return $null
}

function Get-GitHubCliPath {
    $candidates = @(
        'C:\Program Files\GitHub CLI\gh.exe',
        'C:\Program Files (x86)\GitHub CLI\gh.exe'
    )

    if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        $candidates += (Join-Path $env:LOCALAPPDATA 'Programs\GitHub CLI\gh.exe')
    }

    return Get-CommandPath -Name 'gh' -FallbackPaths $candidates
}

function Get-PreferredPowerShellCommand {
    if (Test-CommandExists -Name 'pwsh') {
        return 'pwsh'
    }

    return 'powershell'
}

function ConvertTo-NativeArgumentString {
    param(
        [Parameter(Mandatory)]
        [string[]]$Arguments
    )

    $renderedArguments = foreach ($argument in $Arguments) {
        if ([string]::IsNullOrEmpty($argument)) {
            '""'
            continue
        }

        if ($argument -notmatch '[\s"]') {
            $argument
            continue
        }

        $escaped = $argument -replace '(\\*)"', '$1$1\"'
        $escaped = $escaped -replace '(\\+)$', '$1$1'
        '"' + $escaped + '"'
    }

    return ($renderedArguments -join ' ')
}

function Invoke-GitCapture {
    param(
        [Parameter(Mandatory)]
        [string[]]$Arguments,
        [switch]$AllowFailure
    )

    $stdoutPath = [IO.Path]::GetTempFileName()
    $stderrPath = [IO.Path]::GetTempFileName()

    try {
        $argumentLine = ConvertTo-NativeArgumentString -Arguments $Arguments
        $process = Start-Process -FilePath 'git' `
            -ArgumentList $argumentLine `
            -WorkingDirectory ((Get-Location).Path) `
            -NoNewWindow `
            -Wait `
            -PassThru `
            -RedirectStandardOutput $stdoutPath `
            -RedirectStandardError $stderrPath

        $stdout = if (Test-Path -LiteralPath $stdoutPath) {
            Get-Content -LiteralPath $stdoutPath -Raw -ErrorAction SilentlyContinue
        }
        else {
            ''
        }

        $stderr = if (Test-Path -LiteralPath $stderrPath) {
            Get-Content -LiteralPath $stderrPath -Raw -ErrorAction SilentlyContinue
        }
        else {
            ''
        }

        if ($null -eq $stdout) {
            $stdout = ''
        }

        if ($null -eq $stderr) {
            $stderr = ''
        }

        $output = @($stdout, $stderr) -join ''
        $exitCode = $process.ExitCode

        if (-not $AllowFailure -and $exitCode -ne 0) {
            throw ("git {0} failed with exit code {1}:{2}{3}" -f ($Arguments -join ' '), $exitCode, [Environment]::NewLine, $output.TrimEnd())
        }

        return [PSCustomObject]@{
            ExitCode = $exitCode
            Output   = $output.TrimEnd()
            StdOut   = $stdout.TrimEnd()
            StdErr   = $stderr.TrimEnd()
        }
    }
    finally {
        Remove-Item -LiteralPath $stdoutPath, $stderrPath -Force -ErrorAction SilentlyContinue
    }
}

function Get-CurrentBranch {
    return (Invoke-GitCapture -Arguments @('branch', '--show-current')).Output.Trim()
}

function Assert-CleanWorkingTree {
    $status = (Invoke-GitCapture -Arguments @('status', '--porcelain')).Output.Trim()
    if (-not [string]::IsNullOrWhiteSpace($status)) {
        throw "Working tree is not clean. Commit or stash changes before running this script."
    }
}

function Test-GitRefExists {
    param(
        [Parameter(Mandatory)]
        [string]$Ref
    )

    $result = Invoke-GitCapture -Arguments @('show-ref', '--verify', '--quiet', $Ref) -AllowFailure
    return $result.ExitCode -eq 0
}

function Get-PreferredBaseRef {
    param(
        [string]$BaseBranch = 'main'
    )

    $remoteRef = "refs/remotes/origin/$BaseBranch"
    if (Test-GitRefExists -Ref $remoteRef) {
        return "origin/$BaseBranch"
    }

    return $BaseBranch
}

function Get-FeatureIdFromBranch {
    param(
        [Parameter(Mandatory)]
        [string]$BranchName
    )

    if ($BranchName -match '^(feature|codex|task)/(?<featureId>[A-Za-z0-9._-]+)$') {
        return $Matches['featureId']
    }

    return $null
}

function Resolve-FeatureId {
    param(
        [string]$FeatureId
    )

    if (-not [string]::IsNullOrWhiteSpace($FeatureId)) {
        return $FeatureId
    }

    $branch = Get-CurrentBranch
    $fromBranch = Get-FeatureIdFromBranch -BranchName $branch
    if ([string]::IsNullOrWhiteSpace($fromBranch)) {
        throw "FeatureId was not provided and could not be inferred from branch '$branch'."
    }

    return $fromBranch
}

function Get-FeatureBranchName {
    param(
        [Parameter(Mandatory)]
        [string]$FeatureId,
        [string]$BranchPrefix = 'feature'
    )

    return "$BranchPrefix/$FeatureId"
}

function Get-FeatureMemoryPath {
    param(
        [Parameter(Mandatory)]
        [string]$RepoRoot,
        [Parameter(Mandatory)]
        [string]$FeatureId
    )

    return Join-Path (Join-Path $RepoRoot 'specs') $FeatureId
}

function Assert-FeatureMemory {
    param(
        [Parameter(Mandatory)]
        [string]$RepoRoot,
        [Parameter(Mandatory)]
        [string]$FeatureId
    )

    $featurePath = Get-FeatureMemoryPath -RepoRoot $RepoRoot -FeatureId $FeatureId
    foreach ($requiredName in @('spec.md', 'plan.md', 'tasks.md')) {
        $requiredPath = Join-Path $featurePath $requiredName
        if (-not (Test-Path -LiteralPath $requiredPath)) {
            throw "Required feature-memory file is missing: $requiredPath"
        }
    }

    return $featurePath
}

function Initialize-FeatureMemory {
    param(
        [Parameter(Mandatory)]
        [string]$RepoRoot,
        [Parameter(Mandatory)]
        [string]$FeatureId,
        [Parameter(Mandatory)]
        [string]$BranchName,
        [Parameter(Mandatory)]
        [string]$Title
    )

    $templatesRoot = Join-Path $RepoRoot '.specify\templates'
    $featureRoot = Get-FeatureMemoryPath -RepoRoot $RepoRoot -FeatureId $FeatureId

    if (-not (Test-Path -LiteralPath $featureRoot)) {
        New-Item -ItemType Directory -Path $featureRoot | Out-Null
    }

    $templateMap = @{
        'spec.md'  = Join-Path $templatesRoot 'spec.md'
        'plan.md'  = Join-Path $templatesRoot 'plan.md'
        'tasks.md' = Join-Path $templatesRoot 'tasks.md'
    }

    foreach ($entry in $templateMap.GetEnumerator()) {
        $targetPath = Join-Path $featureRoot $entry.Key
        if (Test-Path -LiteralPath $targetPath) {
            continue
        }

        $content = Get-Content -LiteralPath $entry.Value -Raw -Encoding UTF8
        $content = $content.Replace('{{FEATURE_ID}}', $FeatureId)
        $content = $content.Replace('{{BRANCH_NAME}}', $BranchName)
        $content = $content.Replace('{{TITLE}}', $Title)
        Set-Content -LiteralPath $targetPath -Value $content -Encoding UTF8
    }

    return $featureRoot
}

function Convert-RemoteToHttps {
    param(
        [Parameter(Mandatory)]
        [string]$RemoteUrl
    )

    if ($RemoteUrl -match '^https://') {
        return $RemoteUrl -replace '\.git$', ''
    }

    if ($RemoteUrl -match '^git@github\.com:(?<path>.+?)(\.git)?$') {
        return "https://github.com/$($Matches['path'])"
    }

    return $null
}

function Get-CompareUrl {
    param(
        [Parameter(Mandatory)]
        [string]$BaseBranch,
        [Parameter(Mandatory)]
        [string]$HeadBranch
    )

    $remoteUrl = (Invoke-GitCapture -Arguments @('remote', 'get-url', 'origin')).Output.Trim()
    $httpsUrl = Convert-RemoteToHttps -RemoteUrl $remoteUrl
    if ([string]::IsNullOrWhiteSpace($httpsUrl)) {
        return $null
    }

    return "$httpsUrl/compare/$BaseBranch...$($HeadBranch)?expand=1"
}

function Get-SpecTitle {
    param(
        [Parameter(Mandatory)]
        [string]$FeaturePath
    )

    $specPath = Join-Path $FeaturePath 'spec.md'
    $header = Get-Content -LiteralPath $specPath -Encoding UTF8 | Select-Object -First 1
    if ($header -match '^# Spec:\s+(?<title>.+)$') {
        return $Matches['title'].Trim()
    }

    return Split-Path -Leaf $FeaturePath
}

function Get-ReviewBundlePath {
    param(
        [Parameter(Mandatory)]
        [string]$FeatureId,
        [string]$Prefix = 'review'
    )

    $tempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } elseif ($env:TEMP) { $env:TEMP } else { [IO.Path]::GetTempPath() }
    return Join-Path $tempRoot "$Prefix-$FeatureId.md"
}

function Write-WorkflowSummary {
    param(
        [Parameter(Mandatory)]
        [string]$Text
    )

    if ([string]::IsNullOrWhiteSpace($env:GITHUB_STEP_SUMMARY)) {
        return
    }

    Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY -Value $Text -Encoding UTF8
}
