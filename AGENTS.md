# AGENTS.md

Этот репозиторий использует переносимый process-layer для agent-orchestration. Он отделен от продуктового кода и должен оставаться переносимым в другие Python-репозитории без копирования бизнес-логики текущего приложения.

## Роли

### Orchestrator

- заводит задачу через `specs/<feature-id>/`
- стартует работу только от актуального `main`
- создает одну feature branch на одну задачу
- запускает implementation worker, публикацию PR, CI checks и AI review
- следит, чтобы изменения process-layer сопровождались обновлением `docs/` и `specs/`

### Implementation Agent

- читает `AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и активную папку `specs/<feature-id>/`
- не начинает product-code задачу без активной feature-memory
- держит orchestration-layer вне `src/`, если это не требуется технически
- обновляет `spec.md`, `plan.md` и `tasks.md` по ходу задачи
- не считает локальные незапушенные изменения завершением работы

### AI Reviewer

- работает только внутри PR loop
- оценивает текущий PR head SHA, а не локальную папку разработчика
- не заменяет required checks и не заменяет человека как merge authority
- при отсутствии валидной конфигурации обязан дать понятный fallback, а не имитировать review

### Human

- остается final merge authority
- принимает решение о merge только после green checks, отсутствия merge conflicts и отсутствия blocking findings

## Repository Memory

- `docs/` — durable memory проекта и процесса
- `specs/<feature-id>/` — память конкретной задачи
- `.specify/templates/` — шаблоны для новых feature-memory папок

Product-code задача не стартует, пока не существует активная папка `specs/<feature-id>/` с файлами `spec.md`, `plan.md` и `tasks.md`.

## Инварианты Процесса

- одна задача = одна feature branch = один PR
- старт всегда от актуального `main`
- direct push в `main` запрещен процессом
- product code не считается завершенным вне PR loop
- локальная готовность без PR не считается `done`
- локальные незапушенные изменения не считаются завершением работы
- merge authority остается у человека
- flow не должен зависеть от альтернативной workspace-топологии

## Completion Contract

Задача считается `merge-ready`, только если текущий PR head SHA:

- прошел required checks
- не имеет blocking findings после AI review и human review
- не имеет merge conflicts
- требует только human approval или final merge

## Workspace Policy

Процесс в этом репозитории работает в branch-based модели и не требует отдельной workspace-механики помимо обычной feature branch.

Поэтому текущий process-layer строится на branch-based модели:

- без обязательного альтернативного workspace-path
- без скрытого второго execution path
- с опорой на `feature branch -> PR -> checks -> AI review -> merge-ready`
