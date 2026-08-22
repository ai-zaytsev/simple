# Plan: Spaces Retention Decision

- Feature ID: `009-spaces-retention-decision`
- Feature Branch: `feature/009-spaces-retention-decision`
- Owner: `orchestrator`

## Implementation Slices

1. Записать границу прав в `docs/integrations/digitalocean.md`.
2. Записать решение о retention без расширения прав и его цену.
3. Закрыть задачи стадий 007 и 008.

## Risks

- решение оставляет state Terraform без версий: цена зафиксирована явно, триггер пересмотра назван
- retention, реализованный в задании, ломается вместе с заданием: проверка восстановления должна быть отдельной задачей, а не частью того же скрипта

## Validation Plan

- PR loop зелёный
- перекрёстные ссылки корректны

## Merge Readiness

Документальная задача, закрывается вместе с merge.
