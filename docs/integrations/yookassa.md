# ЮKassa — Разовые Платежи VIP

Первый adapter общего payment provider contract. Архитектурное решение — `ADR-030` в [decisions.md](../architecture/decisions.md). Интеграция использует только [server API ЮKassa](https://yookassa.ru/developers/api) и внешнюю платёжную страницу; мобильного SDK и Google Play Billing нет.

## Границы

```text
Android -- product_id --> Core -- Basic Auth --> ЮKassa
Android <-- HTTPS checkout -- Core <-- payment object -- ЮKassa
                                ^
                                | webhook ID -> authenticated GET -> atomic VIP
```

Android не знает имя провайдера, shop ID, Secret Key, сумму или срок из собственных констант. Core выбирает активный продукт, сохраняет коммерческий snapshot до обращения к провайдеру и создаёт платёж с `capture: true`, `confirmation.type: redirect`, HTTPS `return_url`, внутренним `payment_id` в metadata и стабильным `Idempotence-Key`. Формат и назначение idempotency key описаны в [официальном контракте API](https://yookassa.ru/developers/using-api/interaction-format).

Карточные данные вводятся только на странице ЮKassa. В Core и APK они не поступают.

## Endpoint'ы

| Назначение | Метод и URL | Авторизация |
| --- | --- | --- |
| Создать или продолжить checkout | `POST https://simple-syncbridge.download/v1/payments` | Bearer device token |
| Прочитать сохранённое состояние | `GET https://simple-syncbridge.download/v1/payments/current` | Bearer device token |
| Уведомление ЮKassa | `POST https://simple-syncbridge.download/v1/payments/webhooks/yookassa` | Публичный вход; доверяется только object ID |
| Возврат браузера | `GET https://simple-syncbridge.download/v1/payments/return` | Нет; статичная нейтральная страница |

Webhook body не активирует VIP. По object ID Core выполняет authenticated `GET /v3/payments/{id}` и сверяет provider payment ID, metadata `payment_id`, amount, currency, status и `paid`. Только `succeeded + paid` применяется к entitlement. Повторная доставка возвращает `200`, но `entitlement_applied_at` не позволяет добавить срок второй раз.

## Серверный Каталог

| Product ID | Срок | Цена |
| --- | ---: | ---: |
| `vip_1_month` | 1 месяц | 399 ₽ |
| `vip_3_months` | 3 месяца | 1 090 ₽ |
| `vip_12_months` | 12 месяцев | 3 490 ₽ |

Android передаёт только ID. Сумма в копейках, RUB и длительность в календарных месяцах фиксируются в payment row: изменение каталога не меняет уже созданный платёж.

## Секреты И Контуры

| GitHub Secret | Runtime Core | Где запрещён |
| --- | --- | --- |
| `YOOKASSA_TEST_SHOP_ID` | `CP_YOOKASSA_SHOP_ID` | Android, код, docs, PR, логи |
| `YOOKASSA_TEST_SECRET_KEY` | `CP_YOOKASSA_SECRET_KEY` | Android, код, docs, PR, логи |

`YOOKASSA_TEST_MOBILE_SDK_KEY` не используется. Deploy workflow передаёт два значения непосредственно в `/etc/simple-vpn-core.env` с закрытыми правами. Provider error body отбрасывается, а идентификатор платежа не пишется в журнал.

Test → production требует заменить значения shop ID и Secret Key и повторить deploy. Названия runtime-переменных, API, APK и схема данных одинаковы. До этого перехода должны быть закрыты production-договор, фискализация/чеки, налоговый режим и правила возвратов.

## Настройка Test Webhook

В тестовом магазине ЮKassa настроить один URL:

```text
https://simple-syncbridge.download/v1/payments/webhooks/yookassa
```

Подписать его на `payment.succeeded` и `payment.canceled`. Обработчик также понимает `payment.waiting_for_capture`, хотя при `capture: true` штатный успешный путь сразу приходит в `succeeded`. ЮKassa требует подтверждать уведомление HTTP `200` и повторяет доставку при временном non-200; актуальные события и правила — в [официальной документации webhook](https://yookassa.ru/developers/using-api/webhooks).

Для shop ID + Secret Key webhook настраивается в кабинете тестового магазина. API-регистрация webhook с Bearer token относится к OAuth-сценарию и здесь не используется.

## Живая Test Matrix

[Тестовый режим ЮKassa](https://yookassa.ru/developers/payment-acceptance/testing-and-going-live/testing) не списывает реальные деньги и требует специальные карты. Срок действия — любая будущая дата, CVC и 3-D Secure code — любые числа.

| Проверка | Действие | Обязательный readback |
| --- | --- | --- |
| Успех | `5555 5555 5555 4477`, завершить 3-D Secure | payment `succeeded`, `provider_test = true`, VIP и срок ровно по продукту |
| Неуспех | `5555 5555 5555 4600` (`insufficient_funds`) | payment `canceled`, tier остаётся FREE |
| Пользовательский выход | закрыть/вернуться со страницы до оплаты | VIP не появляется; один возврат не меняет серверный статус |
| Повторный webhook | повторно доставить `payment.succeeded` для того же provider payment ID | HTTP `200`, `vip_expires_at` не сдвигается |
| Подмена полей | unit/integration test меняет amount, currency или metadata при том же status | webhook получает non-200, entitlement не меняется |

После каждого сценария читаются собственные payment row и account tier/expiry из живой PostgreSQL либо operator endpoint. Одной картинки успешной страницы недостаточно.

## Что Не Входит

- подписки и сохранение способа оплаты;
- продление действующего VIP, напоминания и retention-механика;
- возвраты и chargeback automation;
- production receipts и юридическая настройка;
- ручная активация VIP по данным страницы возврата.
