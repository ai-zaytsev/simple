# Plan: ACME Flags Belong To Run

- Feature ID: `026-lego-run-flags`
- Feature Branch: `feature/026-lego-run-flags`
- Owner: `orchestrator`

## Implementation Slices

1. Перенести все опции после `run`.
2. Прогон.

## Risks

- при следующем обновлении клиента набор флагов может снова измениться: версия закреплена, а вывод клиента виден в статусе

## Validation Plan

- локальный рендер
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, подтвердившим оба транспорта.
