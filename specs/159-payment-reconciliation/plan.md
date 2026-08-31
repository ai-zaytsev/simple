# Plan: Каноническая перепроверка платежа

- Feature ID: `159-payment-reconciliation`
- Feature Branch: `feature/159-payment-reconciliation`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать live-дефект и активную feature-memory от merged `main`.
2. Вынести provider reconciliation в единый payment-service path, используемый webhook и ручной проверкой.
3. Разрешить `Current` обращаться к провайдеру только для pending payment с сохранённым provider ID.
4. Обработать provider outage как безопасную ошибку без изменения entitlement; API возвращает временную недоступность, чтобы Android не показывал ложный успех.
5. Покрыть success/cancel/pending/mismatch/outage/exactly-once unit и API тестами.
6. Обновить интеграционную документацию и feature-memory.
7. Пройти PR loop; после human merge развернуть Core/APK и продолжить live payment/refund matrix.

## Risks

- ошибочно принять browser return за оплату вместо authenticated provider GET;
- повторно применить entitlement при одновременном webhook и ручной проверке;
- превратить обычное чтение завершённого платежа в лишний provider API вызов;
- скрыть provider outage за устаревшим `pending` и сообщить пользователю, что проверка состоялась;
- потерять возможность отменить локальный pending после канонического canceled.

## Validation Plan

- unit tests доказывают одинаковую строгую canonical verification для webhook и manual reconcile;
- race-safe exactly-once остаётся на транзакции `ApplySucceeded`;
- API tests проверяют HTTP semantics для provider error и обычного состояния;
- `go test ./...`, `go vet ./...`;
- PR loop на текущем head SHA;
- post-merge deploy, service log, public smoke, panel readback и повтор кнопки на телефоне.

## Merge Readiness

Задача merge-ready только при green required checks, отсутствии blocking findings и конфликтов. Merge выполняет человек. Live-дефект считается закрытым только после выкладки и канонического readback тестового платежа.

## Local Validation Readback

- `go test ./...` — green;
- `go vet ./...` — green;
- `git diff --check` — green;
- отдельный `-race` на Windows недоступен, потому что локальный Go запущен без CGO; это не заменяет и не блокирует штатные CI checks.
