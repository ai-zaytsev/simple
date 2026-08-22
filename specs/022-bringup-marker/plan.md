# Plan: Distinct Bring-up Completion Marker

- Feature ID: `022-bringup-marker`
- Feature Branch: `feature/022-bringup-marker`
- Owner: `orchestrator`

## Implementation Slices

1. Переименовать терминальное состояние в `bringup-complete`.
2. Искать в ожидании именно его.
3. Прогон.

## Risks

- другие промежуточные состояния могут в будущем содержать `complete`: имена состояний задаются в одном файле, и это проверяется чтением
- ожидание в пятнадцать минут может не покрыть медленный выпуск сертификата: при отказе теперь виден последний достигнутый шаг

## Validation Plan

- проверка YAML и ASCII
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, дошедшим до проверок трафика.
