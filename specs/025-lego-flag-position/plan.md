# Plan: ACME Flag Position

- Feature ID: `025-lego-flag-position`
- Feature Branch: `feature/025-lego-flag-position`
- Owner: `orchestrator`

## Implementation Slices

1. Перенести флаг после подкоманды.
2. Прогон.

## Risks

- другие флаги могли переехать так же: вывод клиента теперь виден, поэтому следующая такая ошибка обойдётся в один прогон, а не в несколько

## Validation Plan

- локальный рендер
- прогон

## Merge Readiness

Закрывается прогоном с выпущенным сертификатом.
