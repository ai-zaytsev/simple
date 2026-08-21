# Plan: {{TITLE}}

- Feature ID: `{{FEATURE_ID}}`
- Feature Branch: `{{BRANCH_NAME}}`
- Owner: `orchestrator`

## Implementation Slices

1. Подготовить repository memory и role contracts.
2. Добавить локальный orchestration flow.
3. Добавить GitHub PR-loop workflows.
4. Проверить локальную валидацию и обновить task-memory.

## Risks

- скрытая зависимость от старого процесса
- смешивание process-layer и product logic
- неявная зависимость от локальных CLI или GitHub settings

## Validation Plan

- синтаксическая проверка PowerShell scripts
- проверка process docs/specs на наличие required artifacts
- smoke локального Python validation, если task не требует глубокой продуктовой валидации

## Merge Readiness

Задача считается готовой только после PR loop: green required checks, no blocking findings, no merge conflicts, human approval pending or merge pending.
