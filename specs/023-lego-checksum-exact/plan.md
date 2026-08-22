# Plan: Exact Checksum Match

- Feature ID: `023-lego-checksum-exact`
- Feature Branch: `feature/023-lego-checksum-exact`
- Owner: `orchestrator`

## Implementation Slices

1. Выбирать сумму сравнением поля имени целиком.
2. Прогон.

## Risks

- формат файла сумм может измениться: несовпадение по-прежнему остановит bring-up с понятным статусом

## Validation Plan

- локальный рендер и проверка YAML
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, дошедшим до проверок трафика.
