[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]*$')]
    [string]$FeatureId,

    [Parameter(Mandatory)]
    [string]$Title,

    [string]$BaseBranch = 'main',
    [string]$BranchPrefix = 'feature',
    [switch]$SkipFetch
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir 'Process.Common.ps1')

$repoRoot = Get-RepoRoot
Set-Location -LiteralPath $repoRoot

Assert-CleanWorkingTree

if (-not $SkipFetch) {
    Invoke-GitCapture -Arguments @('fetch', 'origin', $BaseBranch) | Out-Null
}

$branchName = Get-FeatureBranchName -FeatureId $FeatureId -BranchPrefix $BranchPrefix
$baseRef = Get-PreferredBaseRef -BaseBranch $BaseBranch
$existingBranch = (Invoke-GitCapture -Arguments @('branch', '--list', $branchName)).Output.Trim()

if ([string]::IsNullOrWhiteSpace($existingBranch)) {
    Invoke-GitCapture -Arguments @('switch', '--create', $branchName, $baseRef) | Out-Null
}
else {
    Invoke-GitCapture -Arguments @('switch', $branchName) | Out-Null
}

$featurePath = Initialize-FeatureMemory -RepoRoot $repoRoot -FeatureId $FeatureId -BranchName $branchName -Title $Title

Write-Host "Feature branch ready: $branchName"
Write-Host "Feature memory ready: $featurePath"
Write-Host "Next steps:"
Write-Host "  1. Fill spec/plan/tasks in specs/$FeatureId/"
Write-Host "  2. Select implementation agent with scripts/Select-ImplementationAgent.ps1"
Write-Host "  3. Start the worker with scripts/Start-ImplementationWorker.ps1"
