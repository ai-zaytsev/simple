# Specify Templates

`.specify/` stores portable process-memory templates for new tasks.

## How To Use

1. Create a new task with `powershell -File .\scripts\New-FeatureBranch.ps1 -FeatureId <feature-id> -Title "<title>"`.
2. The script creates `specs/<feature-id>/` and fills it from `.specify/templates/`.
3. Fill `spec.md`, `plan.md`, and `tasks.md` before implementation work starts.
4. Do not start product-code changes until feature-memory is active and describes the task scope.
5. After implementation work, run `scripts/Publish-FeaturePR.ps1` so the scripted flow can create a commit, push the branch, and create or update the PR.

## Invariants

- templates do not contain project business logic
- templates assume a branch-based flow without an alternate workspace mechanism
- process-layer changes must be accompanied by `docs/` and `specs/` updates
