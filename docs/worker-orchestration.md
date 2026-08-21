# Worker Orchestration

This document describes the local branch-based flow for Windows PowerShell without any required alternate workspace mechanism.

## Key Rules

- one task = one feature branch = one PR
- always start from the current `main`
- direct push to `main` is forbidden by process
- implementation work does not start without `specs/<feature-id>/`
- process-layer changes stay separate from runtime or product code unless technically required

## Local Flow

1. Create the feature branch and feature-memory:
   `powershell -File .\scripts\New-FeatureBranch.ps1 -FeatureId 000-my-feature -Title "My feature"`
2. Select the implementation agent:
   `powershell -File .\scripts\Select-ImplementationAgent.ps1 -FeatureId 000-my-feature -Agent codex`
3. Prepare or launch the implementation worker:
   `powershell -File .\scripts\Start-ImplementationWorker.ps1 -FeatureId 000-my-feature -Agent codex`
4. Publish the branch and create or update the PR:
   `powershell -File .\scripts\Publish-FeaturePR.ps1 -FeatureId 000-my-feature`
5. Prepare local AI review or a review run:
   `powershell -File .\scripts\Invoke-AIReview.ps1 -FeatureId 000-my-feature -Mode prepare`

If `pwsh` is available in the local environment, the same commands can be run with `pwsh -File ...`.

## What The Scripts Do

- `New-FeatureBranch.ps1` creates a feature branch from the current `main` and bootstraps `specs/<feature-id>/`.
- `Select-ImplementationAgent.ps1` records how the implementation worker should be launched and prints the next runnable command for the current shell environment.
- `Start-ImplementationWorker.ps1` prepares the prompt and context bundle for the implementation agent.
- `Publish-FeaturePR.ps1` creates a commit from local task changes when needed, pushes the branch, and uses GitHub CLI to create or update the PR when `gh` is available and authenticated.
- `Invoke-AIReview.ps1` prepares the review bundle and can launch the selected reviewer when a local review command is configured.

## Why This Repo Uses Branch-Based Flow Only

The versioned files in this repository do not rely on a separate workspace topology, so the supported flow remains a single branch-based working directory for the active task. That flow covers:

- active feature-memory
- dedicated feature branch
- PR as the completion contract
- CI and AI review on the PR head SHA

## Durable vs Task Memory

- `docs/` stores durable process rules and contracts
- `specs/<feature-id>/` stores task memory until the task reaches `merge-ready`
