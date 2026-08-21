# CLAUDE.md

Этот файл фиксирует contract для implementation agent. Его можно использовать как для Claude, так и как шаблон для любой другой агентной системы.

## Перед стартом

1. Определи активный `feature-id`.
2. Прочитай:
   - `AGENTS.md`
   - `docs/worker-orchestration.md`
   - `docs/ai-pr-workflow.md`
   - `specs/<feature-id>/spec.md`
   - `specs/<feature-id>/plan.md`
   - `specs/<feature-id>/tasks.md`
3. Убедись, что работа идет в feature branch, созданной от актуального `main`.

## Обязательные Правила

- Не начинай product-code задачу без активной папки `specs/<feature-id>/`.
- Не считай задачу завершенной по локальному состоянию ветки.
- Не пушь напрямую в `main`.
- Не строй workflow вокруг отдельной workspace-механики.
- Не смешивай orchestration-only изменения с runtime/product кодом без технической необходимости.
- Если меняется process-layer, обновляй `docs/` и `specs/`.

## Что Нужно Поддерживать В Актуальном Виде

- `spec.md` — цель, scope, ограничения, критерии приемки
- `plan.md` — очередность внедрения, риски, валидация
- `tasks.md` — фактическое состояние выполнения

## Definition Of Done

Работа implementation agent заканчивается не на локальном коммите, а на состоянии, при котором:

- feature branch запушена
- PR существует и отражает текущий head SHA
- required checks зеленые
- AI review отработал или явно переключился в documented fallback
- у человека остается только approval или final merge

## Handoff

В handoff нужно отразить:

- что изменено в process-layer
- какие docs/specs обновлены
- как запускать flow локально
- что требуется от GitHub settings или локальной среды
