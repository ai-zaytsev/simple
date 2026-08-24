# Plan: Keep Status Visible After TLS Switch

- Feature ID: `029-status-after-tls`
- Feature Branch: `feature/029-status-after-tls`
- Owner: `orchestrator`

## Implementation Slices

1. Опрашивать статус по HTTPS на домене при редиректе с порта 80.
2. Тот же порядок для итогового вывода.
3. Прогон.

## Risks

- статус остаётся доступным по HTTPS на проверочной ноде: он публикуется только при включённом режиме отладки и никогда в проде

## Validation Plan

- проверка YAML и ASCII
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, дошедшим до четырёх проверок.
