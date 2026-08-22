# Plan: Nginx Map Duplicate Key

- Feature ID: `028-nginx-map-duplicate`
- Feature Branch: `feature/028-nginx-map-duplicate`
- Owner: `orchestrator`

## Implementation Slices

1. Убрать дублирующее значение из `map`.
2. Прогон.

## Risks

- регистронезависимость `map` относится к строковым значениям: при переходе на регулярное выражение поведение изменится

## Validation Plan

- локальный рендер
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, подтвердившим оба транспорта.
