# AI PR Workflow

Этот документ описывает completion contract для branch-based agent flow:

`task -> feature memory -> implementation agent -> PR -> CI checks -> AI review -> merge-ready`

## Required Checks

Для PR loop предусмотрены стабильные названия required checks:

- `Process Baseline`
- `PR Loop Guard`
- `AI Review`

Именно эти проверки должны быть помечены required в настройках репозитория.

## PR Loop Contract

Task считается завершенной только если текущий PR head SHA:

- имеет зеленые required checks
- не имеет blocking findings
- не имеет merge conflicts
- ожидает только human approval или final merge

Локальная ветка без PR не считается завершением задачи.

## AI Reviewer Selection

Workflow читает repo variable `AI_REVIEW_AGENT`.

Поддерживаемые режимы:

- `disabled`
- `manual`
- `codex`
- `claude`
- `custom`

## Fallback Behavior

Если `AI_REVIEW_AGENT`:

- не задан
- содержит неподдерживаемое значение
- требует локальный CLI, которого нет в среде runner

workflow не имитирует review. Вместо этого он:

- публикует понятный summary о fallback
- переводит review authority обратно к human reviewer
- сохраняет предсказуемый check name `AI Review`

## Что Проверяет PR Guard

`PR Loop Guard` валидирует process contract, а не бизнес-логику приложения:

- PR направлен в `main`
- head branch не равен `main`
- для feature branch существует `specs/<feature-id>/`
- в feature-memory присутствуют `spec.md`, `plan.md`, `tasks.md`
- изменения process-layer сопровождаются изменениями `docs/` или `specs/`
- PR не находится в состоянии merge conflict

## Human Authority

Даже после зеленых checks merge authority остается у человека. AI review не делает auto-merge и не заменяет human approval.
