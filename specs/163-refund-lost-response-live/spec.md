# Spec: Живая проверка потерянного ответа возврата

- Feature ID: `163-refund-lost-response-live`
- Feature Branch: `feature/163-refund-lost-response-live`
- Status: `live-accepted`

## Goal

Повторить в test store точный POST уже успешного возврата с тем же
`Idempotence-Key` и доказать, что ЮKassa возвращает тот же provider refund,
денежная операция остаётся одна, а Core/VIP не меняются.

## Non-Goals

- не создавать новый возврат и не менять refund/VIP runtime logic;
- не повторять POST после 24-часового provider window;
- не выводить shop secret, idempotency key или provider object IDs;
- не считать этим тестом недетерминированный refund `insufficient_funds`.

## Scope And Safety

- manual CI workflow и redacted Python verifier;
- только однозначный account prefix, `yookassa`, `provider_test=true`, один
  `succeeded` refund и одна `succeeded` attempt;
- attempt должен быть моложе 24 часов;
- exact original amount/currency/payment/refund metadata и idempotency key
  берутся из private PostgreSQL snapshot;
- POST response и provider list должны указывать на тот же refund; before/after
  durable snapshot обязан совпасть.

## Repository Memory

Обновляются YooKassa integration/runbook, tech debt и feature tasks. Runtime,
Android и schema не меняются. Альтернативная workspace-механика не нужна:
branch-based flow проверен по `AGENTS.md` и `docs/worker-orchestration.md`.

## Acceptance Criteria

- повторный POST получает HTTP 200 и тот же provider refund ID;
- provider list содержит ровно один объект с нашим internal refund metadata;
- tier остаётся FREE, VIP expiry отсутствует, refund/attempt count и сумма не
  меняются;
- safe output содержит только internal 8-char prefixes;
- PR проходит required checks; после merge выполняются live workflow и панель.

## Validation

- unit tests verifier/redaction/production/age/duplicate guards;
- существующие payment replay и acceptance tests;
- PostgreSQL Lifecycle, Process Baseline, PR Loop Guard, AI Review.
