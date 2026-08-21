[CmdletBinding()]
param(
    [string]$FeatureId,
    [string]$BaseBranch = 'main',

    [ValidateSet('prepare', 'local', 'workflow')]
    [string]$Mode = 'prepare',

    [string]$Reviewer,
    [string]$CommandTemplate
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir 'Process.Common.ps1')

$repoRoot = Get-RepoRoot
Set-Location -LiteralPath $repoRoot

$branch = Get-CurrentBranch
$resolvedFeatureId = $null
try {
    $resolvedFeatureId = Resolve-FeatureId -FeatureId $FeatureId
}
catch {
    $resolvedFeatureId = if ([string]::IsNullOrWhiteSpace($FeatureId)) { 'unknown-feature' } else { $FeatureId }
}

$featurePath = $null
if ($resolvedFeatureId -ne 'unknown-feature') {
    try {
        $featurePath = Assert-FeatureMemory -RepoRoot $repoRoot -FeatureId $resolvedFeatureId
    }
    catch {
        $featurePath = $null
    }
}

$baseRef = Get-PreferredBaseRef -BaseBranch $BaseBranch
$headSha = (Invoke-GitCapture -Arguments @('rev-parse', 'HEAD')).Output.Trim()
$diffStat = (Invoke-GitCapture -Arguments @('diff', '--stat', "$baseRef...HEAD")).Output
$changedFiles = (Invoke-GitCapture -Arguments @('diff', '--name-only', "$baseRef...HEAD")).Output
$reviewerName = if (-not [string]::IsNullOrWhiteSpace($Reviewer)) { $Reviewer } elseif (-not [string]::IsNullOrWhiteSpace($env:AI_REVIEW_AGENT)) { $env:AI_REVIEW_AGENT } else { 'disabled' }
$bundlePath = Get-ReviewBundlePath -FeatureId $resolvedFeatureId -Prefix 'ai-review'

$bundleLines = @(
    '# AI Review Bundle',
    '',
    ('- Mode: `{0}`' -f $Mode),
    ('- Reviewer: `{0}`' -f $reviewerName),
    ('- Branch: `{0}`' -f $branch),
    ('- Head SHA: `{0}`' -f $headSha),
    ('- Base Ref: `{0}`' -f $baseRef),
    ('- Feature ID: `{0}`' -f $resolvedFeatureId),
    '',
    '## Changed Files',
    '',
    '```text',
    $changedFiles,
    '```',
    '',
    '## Diff Stat',
    '',
    '```text',
    $diffStat,
    '```'
)

if ($featurePath) {
    $bundleLines += @(
        '',
        '## Feature Memory',
        '',
        ('- Spec: `{0}\spec.md`' -f $featurePath),
        ('- Plan: `{0}\plan.md`' -f $featurePath),
        ('- Tasks: `{0}\tasks.md`' -f $featurePath)
    )
}

Set-Content -LiteralPath $bundlePath -Value ($bundleLines -join [Environment]::NewLine) -Encoding UTF8

$supportedReviewers = @('disabled', 'manual', 'codex', 'claude', 'custom')
$status = 'prepared'
$message = ''

if ($supportedReviewers -notcontains $reviewerName) {
    $status = 'fallback'
    $message = "AI_REVIEW_AGENT='$reviewerName' is not supported. Falling back to manual review."
}
elseif ($reviewerName -in @('disabled', 'manual')) {
    $status = 'fallback'
    $message = "AI review is intentionally set to '$reviewerName'. Human review remains the active review authority."
}
elseif ($Mode -eq 'prepare') {
    $status = 'prepared'
    $message = "Review bundle prepared at $bundlePath. Execute a local reviewer or use the GitHub workflow to continue."
}
else {
    $templateToRun = if (-not [string]::IsNullOrWhiteSpace($CommandTemplate)) {
        $CommandTemplate
    }
    elseif (-not [string]::IsNullOrWhiteSpace($env:AI_REVIEW_COMMAND)) {
        $env:AI_REVIEW_COMMAND
    }
    else {
        $null
    }

    if ([string]::IsNullOrWhiteSpace($templateToRun)) {
        $status = 'fallback'
        $message = "Reviewer '$reviewerName' was selected, but no AI_REVIEW_COMMAND template is configured. Falling back to manual review."
    }
    else {
        $renderedCommand = $templateToRun.Replace('{prompt}', ('"{0}"' -f $bundlePath)).Replace('{featureId}', $resolvedFeatureId)
        Write-Host "Launching AI review with configured template..."
        Invoke-Expression $renderedCommand
        $status = 'executed'
        $message = "AI review command executed with bundle $bundlePath."
    }
}

Write-Host $message
Write-Host "Bundle: $bundlePath"

if ($Mode -eq 'workflow') {
    $summaryLines = @(
        '## AI Review',
        '',
        ('- Status: `{0}`' -f $status),
        ('- Reviewer: `{0}`' -f $reviewerName),
        "- Message: $message",
        ('- Bundle: `{0}`' -f $bundlePath)
    )
    Write-WorkflowSummary -Text ($summaryLines -join [Environment]::NewLine)
}

[PSCustomObject]@{
    Status     = $status
    Reviewer   = $reviewerName
    BundlePath = $bundlePath
    Message    = $message
} | ConvertTo-Json -Depth 3
