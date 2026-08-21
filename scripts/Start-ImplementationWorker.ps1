[CmdletBinding()]
param(
    [string]$FeatureId,

    [ValidateSet('codex', 'claude', 'manual', 'custom')]
    [string]$Agent = 'manual',

    [string]$CommandTemplate,
    [string]$OutputPath,
    [switch]$Execute
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir 'Process.Common.ps1')

$repoRoot = Get-RepoRoot
Set-Location -LiteralPath $repoRoot

$resolvedFeatureId = Resolve-FeatureId -FeatureId $FeatureId
$featurePath = Assert-FeatureMemory -RepoRoot $repoRoot -FeatureId $resolvedFeatureId
$branch = Get-CurrentBranch

if ($branch -eq 'main') {
    throw "Implementation work must run from a feature branch, not from main."
}

$resolvedOutputPath = if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    Get-ReviewBundlePath -FeatureId $resolvedFeatureId -Prefix 'implementation-prompt'
}
else {
    $OutputPath
}

$templateToRun = if (-not [string]::IsNullOrWhiteSpace($CommandTemplate)) {
    $CommandTemplate
}
elseif (-not [string]::IsNullOrWhiteSpace($env:IMPLEMENTATION_AGENT_COMMAND)) {
    $env:IMPLEMENTATION_AGENT_COMMAND
}
else {
    $null
}

$prompt = @"
# Implementation Worker Prompt

- Agent: `$Agent`
- Feature ID: `$resolvedFeatureId`
- Branch: `$branch`

Read the repository contracts and active feature-memory before editing:

- `$repoRoot\AGENTS.md`
- `$repoRoot\CLAUDE.md`
- `$repoRoot\docs\worker-orchestration.md`
- `$repoRoot\docs\ai-pr-workflow.md`
- `$featurePath\spec.md`
- `$featurePath\plan.md`
- `$featurePath\tasks.md`

Execution rules:

- Do not start product-code work without the active feature-memory above.
- Keep orchestration-only changes outside `src/` unless technically required.
- Update `spec.md`, `plan.md` and `tasks.md` as the task evolves.
- Do not rely on alternate workspace layouts.
- One task maps to one feature branch and one PR.
- The task is not done until the PR loop is green and merge-ready.

Deliverable:

- Push the feature branch.
- Create or update the PR.
- Leave the task in a state where only human approval or final merge remains.
"@

Set-Content -LiteralPath $resolvedOutputPath -Value $prompt -Encoding UTF8

Write-Host "Implementation prompt bundle written to: $resolvedOutputPath"

if (-not $Execute) {
    Write-Host "Execute was not requested. Review the prompt and launch the selected agent manually or rerun with -Execute."
    exit 0
}

if ([string]::IsNullOrWhiteSpace($templateToRun)) {
    Write-Host "No implementation agent command template is configured."
    Write-Host "Set IMPLEMENTATION_AGENT_COMMAND or pass -CommandTemplate with a {prompt} placeholder."
    exit 0
}

$renderedCommand = $templateToRun.Replace('{prompt}', ('"{0}"' -f $resolvedOutputPath)).Replace('{featureId}', $resolvedFeatureId)
Write-Host "Launching implementation worker with configured template..."
Invoke-Expression $renderedCommand
