# Plan: Spaces Permission Probe

- Feature ID: `008-spaces-permission-probe`
- Feature Branch: `feature/008-spaces-permission-probe`
- Owner: `orchestrator`

## Implementation Slices

1. Сделать шаги настройки bucket неразрушающими.
2. Довести round-trip проверку до выполнения при любом исходе.
3. Запустить и зафиксировать фактическую границу прав.
4. Вынести решение о запросе прав Business Owner.

## Risks

- неразрушающие шаги могут скрыть проблему, если исход не попадает в summary: поэтому исход каждой попытки печатается отдельной строкой таблицы
- вариант с retention внутри бэкапного задания оставляет state Terraform без версий: это фиксируется как принятое ограничение, а не умалчивается

## Validation Plan

- YAML-валидация, проверка ASCII
- запуск с `apply=true` после merge
- сверка: round-trip выполнен, исходы настройки bucket зафиксированы

## Merge Readiness

Задача закрывается, когда граница прав известна и записана.
