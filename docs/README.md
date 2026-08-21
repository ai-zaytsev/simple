# Документация Проекта

Проект: приложение VPN. На текущем этапе репозиторий содержит только process-layer (agent orchestration). Стек MVP зафиксирован в `docs/stack.md`, продуктовая архитектура и продуктовый код ещё не проектировались.

`docs/` — durable memory процесса:

- `docs/ai-pr-workflow.md` — completion contract через PR loop
- `docs/worker-orchestration.md` — локальный branch-based orchestration flow
- `docs/environment.md` — состояние локальной среды и требования к ней
- `docs/stack.md` — зафиксированный стек MVP и его следствия для process-layer

Связанный task-memory layer хранится в `specs/<feature-id>/`. Product-code задача не должна стартовать без активной feature-memory папки с `spec.md`, `plan.md` и `tasks.md`.

## Правило Наполнения

В `docs/` попадают только документы, соответствующие текущему состоянию репозитория. Продуктовые разделы (`overview/`, `api/`, `product/`, `deployment/`, `infra/`, `security/`) создаются позже — в момент, когда появляется соответствующий код или принятое решение, а не заранее.

Что сюда намеренно не кладётся:

- audit / gap-analysis / greenfield-analysis
- transition-документы, описывающие незавершённый переход как факт
- корневые markdown-файлы вне `docs/`, кроме `AGENTS.md` и `CLAUDE.md`
