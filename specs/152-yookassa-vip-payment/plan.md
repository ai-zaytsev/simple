# Plan: Оплата VIP через ЮKassa

- Feature ID: `152-yookassa-vip-payment`
- Feature Branch: `feature/152-yookassa-vip-payment`
- Owner: `orchestrator`

## Срезы Реализации

1. Зафиксировать каталог продуктов, unit economics, provider-neutral доменную модель и границы секретов.
2. Добавить миграцию продуктов, платежей и срока VIP; сохранить административный VIP без срока.
3. Реализовать общий payment provider contract и HTTP adapter ЮKassa с Basic Auth, redirect checkout и idempotency key.
4. Добавить Store операции для создания/повтора платежа, canonical status и атомарного однократного применения entitlement.
5. Добавить аутентифицированные Core endpoints создания/чтения платежа и публичный webhook, который подтверждает состояние через API ЮKassa.
6. Подключить runtime-конфигурацию и существующие GitHub Secrets только к Core deployment.
7. Передать каталог Android, открыть checkout системным браузером и перечитать серверный статус после возврата; не добавлять SDK.
8. Добавить статичную страницу возврата в публичный Core route, которая не утверждает, что платеж успешен и не меняет состояние.
9. Покрыть unit, HTTP, schema/source-contract и Android tests; обновить durable docs.
10. Опубликовать PR, пройти checks/review, после human merge развернуть Core, настроить webhook тестового магазина и выполнить live test matrix.

## Риски И Контроли

| Риск | Контроль |
| --- | --- |
| Поддельный webhook | Webhook не доверяется; платеж перечитывается серверным API и полностью сверяется |
| Двойное продление | `entitlement_applied_at` под row lock и одна транзакция с обновлением аккаунта |
| Двойной checkout из-за повторов путей Control Plane | Собственный payment row до provider call, стабильный idempotency key, один открытый платеж на аккаунт |
| Цена/срок подменены APK | Android передает только product ID, Core читает серверный snapshot |
| VIP остается после срока | `vip_expires_at`, очистка до начала HTTP и периодический reaper |
| Секрет попадает в APK/логи | Только env Core/CI, sanitized provider errors, source checks |
| Webhook приходит между create API и локальной записью | Локальная запись создается первой; non-200 просит ЮKassa повторить доставку |
| Возврат из браузера принят за оплату | Return page не меняет состояние; Android читает только Core, Core меняется только после canonical provider status |
| Production требует receipt/налоговых данных | Не входит в test-store stage; записывается явным tech debt до реальных продаж |

## Валидация

- `go test ./...`, `go vet ./...`, `go mod tidy` без diff;
- mock HTTP server tests adapter ЮKassa: auth, payload, idempotency, sanitized errors;
- payment decision tests: success, canceled, pending, duplicate webhook, amount/metadata mismatch;
- migration/schema tests: unique provider ID, one open payment, payment history, expiring entitlement;
- Android unit/source-contract tests и CI Android build;
- repository secret-name check без чтения значений;
- PR required checks `Process Baseline`, `PR Loop Guard`, `AI Review`;
- после deploy — тестовые success/failure/cancel и повтор webhook, затем live panel/account readback.

## Merge Readiness

Только текущий PR head SHA с green required checks, без blocking findings и merge conflicts. Merge делает человек. Стадия закрывается после post-deploy live verification, а не после локальных тестов.
