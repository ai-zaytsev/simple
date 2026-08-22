# Plan: Capture ACME Client Output

- Feature ID: `024-acme-diagnostics`
- Feature Branch: `feature/024-acme-diagnostics`
- Owner: `orchestrator`

## Implementation Slices

1. Писать вывод ACME-клиента в файл.
2. При отказе добавлять последние строки в статус.
3. Перечитывать статус при неуспехе ожидания.
4. Прогон.

## Risks

- вывод клиента может содержать адрес аккаунта: он на нашем домене, а не личный
- статус публикуется только на проверочных нодах

## Validation Plan

- локальный рендер и проверка YAML
- прогон

## Merge Readiness

Закрывается прогоном, в котором либо выпущен сертификат, либо видна причина отказа.
