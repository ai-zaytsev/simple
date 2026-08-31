# Plan: Безопасная повторная доставка webhook ЮKassa

- Feature ID: `162-payment-webhook-replay`
- Feature Branch: `feature/162-payment-webhook-replay`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать test-only и redaction contracts в feature memory.
2. Реализовать replayer/validator с проверкой private before/after snapshots.
3. Добавить manual workflow, который получает snapshot через защищённый SSH,
   повторяет webhook и удаляет временные файлы при любом исходе.
4. Добавить unit tests и включить их в Process Baseline.
5. Обновить BO/runbook и integration memory, выполнить PR loop.
6. После human merge запустить live replay, прочитать панель и записать verdict.

## Risks

- утечка provider ID через аргументы или лог — private snapshot остаётся файлом,
  Python никогда не печатает полный ID;
- replay старого payment после refund повторно выдаст VIP — workflow считает
  любой `applied=true` и любое изменение entitlement блокирующей ошибкой;
- выбран не тот аккаунт/платёж — prefix и test-store predicates fail closed;
- webhook route отвечает `200`, но меняет состояние — before/after comparison
  проверяет не только HTTP, но и durable PostgreSQL result.

## Validation Plan

- happy-path unit test с четырьмя webhook вызовами;
- отказ при production payment, неоднозначном аккаунте, `applied=true`, non-200
  и изменении refund/tier/timestamps;
- проверка отсутствия полных IDs и private marker в safe report;
- required GitHub checks на текущем PR head.

## Merge Readiness

Задача merge-ready только после green required checks, отсутствия blocking
findings/conflicts и human merge authority. Live acceptance выполняется уже на
merged `main`; без readback панели стадия не закрывается.
