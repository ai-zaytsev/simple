[CmdletBinding()]
param(
    [string]$FeatureId,

    [Parameter(Mandatory)]
    [ValidateSet('codex', 'claude', 'manual', 'custom')]
    [string]$Agent,

    [string]$CustomCommand,
    [switch]$AsJson
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir 'Process.Common.ps1')

$repoRoot = Get-RepoRoot
Set-Location -LiteralPath $repoRoot

$resolvedFeatureId = Resolve-FeatureId -FeatureId $FeatureId
$featurePath = Assert-FeatureMemory -RepoRoot $repoRoot -FeatureId $resolvedFeatureId
$branch = Get-CurrentBranch
$shellCommand = Get-PreferredPowerShellCommand

$contracts = @('AGENTS.md', 'CLAUDE.md', "specs/$resolvedFeatureId/spec.md", "specs/$resolvedFeatureId/plan.md", "specs/$resolvedFeatureId/tasks.md")
$launchTemplate = if (-not [string]::IsNullOrWhiteSpace($CustomCommand)) {
    $CustomCommand
}
elseif (-not [string]::IsNullOrWhiteSpace($env:IMPLEMENTATION_AGENT_COMMAND)) {
    $env:IMPLEMENTATION_AGENT_COMMAND
}
else {
    $null
}

$result = [PSCustomObject]@{
    featureId          = $resolvedFeatureId
    branch             = $branch
    featureMemoryPath  = $featurePath
    agent              = $Agent
    contracts          = $contracts
    launchTemplate     = $launchTemplate
    supportsAutoLaunch = -not [string]::IsNullOrWhiteSpace($launchTemplate)
    nextStep           = "$shellCommand -File .\scripts\Start-ImplementationWorker.ps1 -FeatureId $resolvedFeatureId -Agent $Agent"
}

if ($AsJson) {
    $result | ConvertTo-Json -Depth 4
    exit 0
}

Write-Host "Implementation agent selected: $Agent"
Write-Host "Feature: $resolvedFeatureId"
Write-Host "Branch: $branch"
Write-Host "Read before execution:"
foreach ($contract in $contracts) {
    Write-Host "  - $contract"
}

if ($result.supportsAutoLaunch) {
    Write-Host "Auto-launch template is available."
}
else {
    Write-Host "Auto-launch template is not configured. The next script will prepare a prompt bundle."
}

Write-Host "Next:"
Write-Host "  $($result.nextStep)"
