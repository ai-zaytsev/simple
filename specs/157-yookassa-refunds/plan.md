# Plan: Проверка ЮKassa и возвраты VIP

- Feature ID: `157-yookassa-refunds`
- Feature Branch: `feature/157-yookassa-refunds`
- Owner: `orchestrator`

## Срезы Реализации

1. Проверить текущую модель платежей, entitlement и Android-flow, а также актуальные официальные ограничения refunds API ЮKassa.
2. Зафиксировать provider-neutral доменную модель, точную математику возврата и конечный автомат состояний.
3. Добавить миграции для снимка оплаченного периода, логического возврата и истории provider attempts.
4. Расширить payment provider contract и реализовать YooKassa create/get refund adapter без утечки секретов.
5. Реализовать атомарное резервирование, сверку и применение возврата в Core; для потери ответа сначала искать provider refund по исходному payment/metadata, затем повторять POST только внутри provider idempotency window.
6. Добавить аутентифицированный Core API и безопасный Android-flow с явным подтверждением пользователя.
7. Покрыть policy, store, API, provider и Android тестами все положительные, ошибочные и граничные сценарии.
8. Обновить durable docs, проверить исходные способы и поддержку частичного возврата.
9. Опубликовать PR и пройти checks/review.
10. После human merge развернуть Core/APK и вместе с Business Owner выполнить live test-store matrix и post-deploy readback.

## Риски И Контроли

| Риск | Контроль |
| --- | --- |
| Двойное движение денег | Логический idempotency key, история попыток и уникальные ограничения БД |
| VIP выключен до возврата денег | Entitlement меняется только в транзакции после канонического `succeeded` |
| Потерян ответ ЮKassa | Повтор той же попытки с тем же ключом и последующая canonical сверка |
| Старый провайдер недоступен после переключения продаж | Выбор adapter по provider исходного платежа, а не по текущему provider продаж |
| Ошибка pro rata и округления | Целочисленная математика в копейках, UTC timestamps, таблица boundary tests |
| Provider не поддерживает частичный возврат | Явный neutral отказ; без обхода и без отключения VIP |
| Provider minimum выше рассчитанной суммы | Не округлять в пользу или против пользователя; операция не исполняется автоматически |
| Повтор webhook | Сверка provider ID/payment ID и идемпотентное применение под блокировкой БД |

## Валидация

- `go test ./...`, `go vet ./...`, `go mod tidy` без diff;
- чистый PostgreSQL migration/store lifecycle в отдельной PR-проверке `PostgreSQL Lifecycle`;
- mock provider tests: full/partial, status, canceled/error, insufficient funds, lost response, idempotent retry;
- policy tests на точной границе 7 дней и в конце периода;
- API/auth/account ownership tests;
- Android unit/source-contract/build tests;
- live test-store: payment success/failure/cancel, webhook/repeat, full and partial refunds, provider status and VIP readback;
- PR checks `Process Baseline`, `PR Loop Guard`, `AI Review`.

## Merge Readiness

Только текущий PR head SHA с green required checks, без blocking findings и merge conflicts. Merge делает человек; стадия закрывается после live test-store и post-deploy сверки.
