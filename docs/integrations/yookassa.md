# ЮKassa — Разовые Платежи И Возвраты VIP

Первый adapter общего payment provider contract. Архитектурные решения — `ADR-030` и `ADR-032` в [decisions.md](../architecture/decisions.md). Интеграция использует только [server API ЮKassa](https://yookassa.ru/developers/api) и внешнюю платёжную страницу; мобильного SDK и Google Play Billing нет.

## Границы

```text
Android -- product_id --> Core -- Basic Auth --> ЮKassa
Android <-- HTTPS checkout -- Core <-- payment object -- ЮKassa
                                ^
                                | webhook ID -> authenticated GET -> atomic VIP

Android -- payment_id --> Core -- refund amount + idempotency --> provider of original payment
Android <-- neutral state -- Core <-- canonical refund/list/GET ---- provider
```

Android не знает имя провайдера, shop ID, Secret Key, сумму или срок из собственных констант. Core выбирает активный продукт, сохраняет коммерческий snapshot до обращения к провайдеру и создаёт платёж с `capture: true`, `confirmation.type: redirect`, HTTPS `return_url`, внутренним `payment_id` в metadata и стабильным `Idempotence-Key`. Формат и назначение idempotency key описаны в [официальном контракте API](https://yookassa.ru/developers/using-api/interaction-format).

Карточные данные вводятся только на странице ЮKassa. В Core и APK они не поступают.

## Endpoint'ы

| Назначение | Метод и URL | Авторизация |
| --- | --- | --- |
| Создать или продолжить checkout | `POST https://simple-syncbridge.download/v1/payments` | Bearer device token |
| Прочитать сохранённое состояние | `GET https://simple-syncbridge.download/v1/payments/current` | Bearer device token |
| Рассчитать возврат без движения денег | `POST https://simple-syncbridge.download/v1/refunds/quote` | Bearer device token |
| Создать или безопасно продолжить возврат | `POST https://simple-syncbridge.download/v1/refunds` | Bearer device token |
| Канонически сверить возврат | `POST https://simple-syncbridge.download/v1/refunds/current` | Bearer device token |
| Уведомление ЮKassa | `POST https://simple-syncbridge.download/v1/payments/webhooks/yookassa` | Публичный вход; доверяется только object ID |
| Возврат браузера | `GET https://simple-syncbridge.download/v1/payments/return` | Нет; статичная нейтральная страница |

Webhook body не активирует VIP. По object ID Core выполняет authenticated `GET /v3/payments/{id}` и сверяет provider payment ID, metadata `payment_id`, amount, currency, status и `paid`. Только `succeeded + paid` применяется к entitlement. Повторная доставка возвращает `200`, но `entitlement_applied_at` не позволяет добавить срок второй раз.

Кнопка Android `Проверить оплату` — recovery для потерянного или задержанного webhook, а не второй источник истины. Для сохранённого `pending` Core сам читает тот же payment через authenticated provider API и проводит его через ту же каноническую сверку. Android и страница возврата не передают статус. После `succeeded` обычное чтение больше не обращается к провайдеру, а транзакционный `entitlement_applied_at` защищает от одновременного webhook и нажатия кнопки.

## Возвраты

Политика принадлежит Core и считается в целых копейках:

- от подтверждённой оплаты до границы ровно `7 × 24 часа` включительно — 100% исходной суммы;
- после границы и до конца оплаченного периода — `floor(сумма × оставшийся срок / полный срок × 75%)`;
- после конца оплаченного периода автоматического возврата нет;
- результат ниже provider minimum не округляется вверх.

Android передаёт только внутренний `payment_id` и явное согласие на повтор после канонически отменённой попытки. Core выбирает adapter по `provider` исходного платежа, хранит один логический refund, все provider attempts и отдельный idempotency key каждой попытки. Сумма, валюта, способ оплаты и имя провайдера с телефона не принимаются.

ЮKassa возвращает деньги только на исходный способ оплаты. Минимальный частичный возврат — 1 ₽; сумма всех возвратов не может превысить платёж. До расширения живой матрицы adapter разрешает частичный запрос только для используемого тестами `bank_card`; любой другой способ получает нейтральный отказ без обходного перевода и без изменения VIP. Актуальные provider-правила: [возвраты](https://yookassa.ru/developers/payment-acceptance/after-the-payment/refunds) и [способы оплаты](https://yookassa.ru/developers/payment-acceptance/getting-started/payment-methods).

Ответ создания не считается результатом. Core читает `GET /v3/refunds/{id}` и сверяет refund ID, исходный provider payment ID, внутренний metadata ID, сумму и валюту. Только канонический `succeeded` атомарно завершает refund и запускает общий VIP→FREE transition; `creating`, `pending`, `canceled`, ошибка и потеря ответа VIP не меняют. Правила отзыва VPN-доступа при этом transition описаны в [entitlement model](../architecture/entitlement-model.md#где-применяется-ограничение).

Перед повтором потерянного POST Core запрашивает список возвратов по исходному `payment_id` и ищет собственный metadata ID. ЮKassa гарантирует идемпотентность POST 24 часа; если за это время исход установить не удалось, Core не отправляет новый денежный запрос вслепую. Операция остаётся на ручной сверке, а VIP продолжает работать.

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

Подписать его на `payment.succeeded`, `payment.canceled` и `refund.succeeded`. Обработчик также понимает `payment.waiting_for_capture`, хотя при `capture: true` штатный успешный путь сразу приходит в `succeeded`. Для отменённого refund отдельного события нет, поэтому Core периодически перечитывает незавершённые возвраты. ЮKassa требует подтверждать уведомление HTTP `200` и повторяет доставку при временном non-200; актуальные события и правила — в [официальной документации webhook](https://yookassa.ru/developers/using-api/webhooks).

Для shop ID + Secret Key webhook настраивается в кабинете тестового магазина. API-регистрация webhook с Bearer token относится к OAuth-сценарию и здесь не используется.

## Живая Test Matrix

[Тестовый режим ЮKassa](https://yookassa.ru/developers/payment-acceptance/testing-and-going-live/testing) не списывает реальные деньги и требует специальные карты. Срок действия — любая будущая дата, CVC и 3-D Secure code — любые числа.

Workflow `Payment Acceptance` принимает безопасный account UUID-prefix. Действие `read` получает локальные payment/refund rows и через существующие CI secrets делает authenticated GET тех же объектов у ЮKassa; публично выводятся только внутренние восьмизначные prefixes и проверяемые поля. Действие `prepare_partial` предназначено только для живой test-store матрицы: оно выставляет policy interval последнего подходящего `provider_test=true` карточного платежа в точку 8 суток. Production, non-VIP, неподтверждённый, non-card или уже возвращённый платёж workflow менять не может.

Workflow `Payment Webhook Replay` предназначен для проверки повторной доставки уже завершённой test-store цепочки. Он сам получает private provider IDs из PostgreSQL, дважды повторяет `payment.succeeded` и `refund.succeeded`, требует четыре ответа `HTTP 200 / received=true / applied=false`, затем повторно читает durable state. Full provider IDs не являются input/output и не печатаются. Workflow отказывается работать, если аккаунт не `FREE`, payment/refund не `succeeded`, `provider_test` не равен `true`, возвратов или provider attempts не ровно по одному либо сумма/entitlement изменились.

Workflow `Refund Lost Response` воспроизводит потерю ответа на POST без создания искусственного refund. Только в пределах 24 часов он берёт private idempotency key и exact payload уже успешной test-store attempt, повторяет запрос к ЮKassa и требует тот же provider refund. Затем provider list должен содержать ровно одну операцию с нашим internal refund metadata, а before/after snapshot — остаться неизменным. Key, Basic Auth и provider IDs не выводятся. Это проверяет provider-owned idempotency отдельно от webhook replay.

| Проверка | Действие | Обязательный readback |
| --- | --- | --- |
| Успех | `5555 5555 5555 4477`, завершить 3-D Secure | payment `succeeded`, `provider_test = true`, VIP и срок ровно по продукту |
| Неуспешная попытка карты | `5555 5555 5555 4600` (`insufficient_funds`) | checkout показывает отказ, tier остаётся FREE; redirect checkout может оставить payment `pending` для другой карты |
| Пользовательский выход | закрыть страницу до ввода карты и не открывать payment снова | после часового окна DB/provider `canceled/canceled`, VIP не появляется |
| Повторный webhook | `Payment Webhook Replay` для завершённой test-store цепочки | четыре HTTP `200`, все `applied=false`; tier/expiry/timestamps/refund count и сумма не меняются |
| Подмена полей | unit/integration test меняет amount, currency или metadata при том же status | webhook получает non-200, entitlement не меняется |
| Полный возврат | успешный платёж моложе 7 суток | refund `succeeded`, возвращена вся сумма, VIP и внешние credentials прекращены только после canonical GET |
| Частичный возврат | тестовый paid interval с возрастом больше 7 суток | сумма равна `floor(pro rata × 75%)`, исходный способ принимает частичный refund, VIP и внешние credentials прекращены после `succeeded` |
| Недостаточный баланс | создать refund при недоступной сумме магазина | refund `canceled/insufficient_funds`, VIP остаётся |
| Повтор и потеря ответа | `Payment Webhook Replay`, затем `Refund Lost Response` до 24-часовой границы | один provider refund/attempt, одна сумма и одна смена entitlement |
| Границы | сразу, ровно 7 суток, после 7 суток, почти в конце и после конца | 100%, 100%, pro rata × 75%, минимум/отказ, автоматического возврата нет |

После каждого сценария читаются собственные payment row и account tier/expiry из живой PostgreSQL либо operator endpoint. Одной картинки успешной страницы недостаточно.

### Подтверждено 31 августа — 1 сентября 2026 года

- успешная карточная оплата 399 ₽ канонически активировала VIP; потерянный webhook был восстановлен штатной кнопкой `Проверить оплату` без доверия Android;
- full refund того же `bank_card` платежа вернул 399 ₽, завершился `succeeded` и только после этого перевёл VIP→FREE;
- external credential после refund исчез из node list, обе ноды перешли `count=2→1`, а сохранённая ссылка перестала давать интернет;
- вторая карточная оплата 399 ₽ после одной неуспешной попытки на checkout завершилась `succeeded`, Core и ЮKassa совпали, панель показала `VIP=1`, `FREE=0`;
- partial refund после подготовленной точки 8 суток вернул `222,01 ₽` на исходную `bank_card`, Core и ЮKassa дали `succeeded/succeeded`, entitlement был отозван, панель вернулась к `VIP=0`, `FREE=1`;
- Business Owner настроил test-store webhook на `payment.succeeded`, `payment.canceled` и `refund.succeeded`; два повтора payment и два refund notification получили `HTTP 200 / applied=false`, refund count/amount и VIP не изменились (`Payment Webhook Replay` run `33475060953`);
- exact POST partial refund с исходным idempotency key получил `HTTP 200`, вернул тот же provider refund, а provider list сохранил ровно одну операцию; DB осталась с одним refund/attempt на `222,01 ₽` (`Refund Lost Response` run `33476438881`);
- отдельный checkout, закрытый до ввода карты, через часовое окно стал `canceled/canceled`; DB уже содержала `canceled` до read workflow, при этом аккаунт остался FREE без `paid_at` и entitlement (`Payment Acceptance` run `33480834901`);
- purchases возвращены в продуктовое состояние `open=true`, `FREE=7 дней`; post-merge панель: `FREE=1`, `VIP=0` (`Purchases` run `33480898240`, финальный `Read The Panel` run `33481599092`).

Все воспроизводимые live-сценарии test store завершены. Попытка карты `5555 5555 5555 4600` показала пользователю `insufficient_funds` и не включила VIP, но checkout штатно сохранил payment `pending` для другой карты; terminal cancel поэтому проверен отдельным брошенным checkout. Единственное provider limitation: официальная документация описывает `insufficient_funds` для refund, но не публикует детерминированный test-store trigger. Этот исход не имитировался и не объявляется живым; automated provider-error contract покрыт тестами, незавершённый refund VIP не отключает.

## Что Не Входит

- подписки и сохранение способа оплаты;
- продление действующего VIP, напоминания и retention-механика;
- chargeback automation и спорные операции;
- production receipts и юридическая настройка;
- ручная активация VIP по данным страницы возврата.
