# Spec: ACME Flag Position

- Feature ID: `025-lego-flag-position`
- Feature Branch: `feature/025-lego-flag-position`
- Status: `in-progress`

## Причина

ACME-клиент выходил мгновенно с ошибкой `flag provided but not defined: -accept-tos`. В версии 5 этот флаг принадлежит подкоманде `run`, а не глобальным опциям.

Отказ выглядел как сетевая проблема ровно до того момента, когда вывод клиента попал в статус. Диагностика, добавленная предыдущей задачей, окупилась на первом же прогоне.

## Решение

Флаг перенесён после `run`.

## Acceptance Criteria

- сертификат выпускается
- bring-up доходит до `bringup-complete`

## Validation

- фактический прогон
