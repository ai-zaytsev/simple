[CmdletBinding()]
param(
    [string]$FeatureId,
    [string]$BaseBranch = 'main',
    [string]$Remote = 'origin',
    [string]$Title,
    [string]$CommitMessage,
    [switch]$PushOnly
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir 'Process.Common.ps1')

$repoRoot = Get-RepoRoot
Set-Location -LiteralPath $repoRoot

$branch = Get-CurrentBranch
if ($branch -eq 'main') {
    throw "Publishing a feature PR from main is not allowed."
}

$resolvedFeatureId = Resolve-FeatureId -FeatureId $FeatureId
$featurePath = Assert-FeatureMemory -RepoRoot $repoRoot -FeatureId $resolvedFeatureId
$specTitle = Get-SpecTitle -FeaturePath $featurePath
$prTitle = if ([string]::IsNullOrWhiteSpace($Title)) { "[$resolvedFeatureId] $specTitle" } else { $Title }
$resolvedCommitMessage = if ([string]::IsNullOrWhiteSpace($CommitMessage)) { $prTitle } else { $CommitMessage }
$bodyPath = Get-ReviewBundlePath -FeatureId $resolvedFeatureId -Prefix 'pr-body'
$compareUrl = Get-CompareUrl -BaseBranch $BaseBranch -HeadBranch $branch

$body = @"
## Summary

- Feature ID: `$resolvedFeatureId`
- Branch: `$branch`
- Spec: `specs/$resolvedFeatureId/spec.md`
- Plan: `specs/$resolvedFeatureId/plan.md`
- Tasks: `specs/$resolvedFeatureId/tasks.md`

## Process Contract

- branch-based flow only
- one task = one branch = one PR
- completion is defined by PR loop, not by local branch state

## Validation

- update local validation results before requesting final merge
- keep docs/specs in sync with process-layer changes
"@

Set-Content -LiteralPath $bodyPath -Value $body -Encoding UTF8

$workingTreeStatus = (Invoke-GitCapture -Arguments @('status', '--porcelain')).Output.Trim()
if (-not [string]::IsNullOrWhiteSpace($workingTreeStatus)) {
    Invoke-GitCapture -Arguments @('add', '-A') | Out-Null

    $stagedStatus = (Invoke-GitCapture -Arguments @('diff', '--cached', '--name-only')).Output.Trim()
    if (-not [string]::IsNullOrWhiteSpace($stagedStatus)) {
        Invoke-GitCapture -Arguments @('commit', '-m', $resolvedCommitMessage) | Out-Null
        Write-Host "Commit created: $resolvedCommitMessage"
    }
}

Invoke-GitCapture -Arguments @('push', '--set-upstream', $Remote, $branch) | Out-Null
Write-Host "Branch pushed: $branch"

if ($PushOnly) {
    if ($compareUrl) {
        Write-Host "Create PR manually: $compareUrl"
    }

    exit 0
}

 $ghPath = Get-GitHubCliPath
 if ([string]::IsNullOrWhiteSpace($ghPath)) {
    Write-Host "GitHub CLI was not found. Push succeeded, but PR must be created manually."
    if ($compareUrl) {
        Write-Host "Create PR manually: $compareUrl"
    }

    exit 0
}

$authStatus = & $ghPath auth status 2>&1
if ($LASTEXITCODE -ne 0) {
    throw ("gh auth status failed:{0}{1}" -f [Environment]::NewLine, (($authStatus | Out-String).TrimEnd()))
}

$existingPrJson = & $ghPath pr list --head $branch --base $BaseBranch --json number,url --limit 1 2>$null
if ($LASTEXITCODE -ne 0) {
    throw "gh pr list failed. Authenticate GitHub CLI and try again."
}

$existingPr = $null
if (-not [string]::IsNullOrWhiteSpace($existingPrJson)) {
    $parsed = $existingPrJson | ConvertFrom-Json
    if ($parsed.Count -gt 0) {
        $existingPr = $parsed[0]
    }
}

if ($null -ne $existingPr) {
    & $ghPath pr edit $existingPr.number --title $prTitle --body-file $bodyPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "gh pr edit failed."
    }

    Write-Host "PR updated: $($existingPr.url)"
    exit 0
}

$createOutput = & $ghPath pr create --base $BaseBranch --head $branch --title $prTitle --body-file $bodyPath 2>&1
if ($LASTEXITCODE -ne 0) {
    throw ("gh pr create failed:{0}{1}" -f [Environment]::NewLine, ($createOutput | Out-String).TrimEnd())
}

Write-Host "PR created:"
Write-Host (($createOutput | Out-String).TrimEnd())
