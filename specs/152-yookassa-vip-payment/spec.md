# Spec: Оплата VIP через ЮKassa

- Feature ID: `152-yookassa-vip-payment`
- Feature Branch: `feature/152-yookassa-vip-payment`
- Status: `active`

## Цель

Пользователь FREE, которому сервер уже разрешил покупку, выбирает VIP на 1, 3 или 12 месяцев в Android-приложении, получает от Core внешнюю HTTPS-страницу оплаты ЮKassa и после подтвержденного webhook становится VIP на оплаченный срок.

Источник истины — PostgreSQL Core. Возврат со страницы оплаты, ответ Android или тело webhook сами по себе VIP не выдают: Core повторно читает платеж через серверный API провайдера и применяет подтвержденный платеж атомарно и не более одного раза.

## Продукты И Цена

| Product ID | Срок | Цена | Цена месяца |
| --- | ---: | ---: | ---: |
| `vip_1_month` | 1 месяц | 399 ₽ | 399 ₽ |
| `vip_3_months` | 3 месяца | 1 090 ₽ | 363,33 ₽ |
| `vip_12_months` | 12 месяцев | 3 490 ₽ | 290,83 ₽ |

Цена рассчитана не по временным $9/мес живого DigitalOcean-контура, а по полной уже утвержденной [топологии $35/мес](../../docs/architecture/mvp-topology.md). Для запаса принят курс 100 ₽/$ при [официальном курсе ЦБ около 85,6 ₽/$ на 29.08.2026](https://www.cbr.ru/currency_base/daily/?UniDbQuery.Posted=True&UniDbQuery.To=29.08.2026) и [карточная комиссия ЮKassa 3,5% плюс НДС 22% на комиссию](https://yookassa.ru/docs/support/payments/fees), то есть до 4,27% платежа. При 15 платящих из первых 50 пользователей годовой продукт дает около 4 175 ₽ чистого нормализованного месячного поступления против 3 500 ₽ инфраструктуры: запас около 19%. При текущем курсе запас выше. Налоги конкретного юридического лица в расчёт не включены: договор и налоговый режим не зафиксированы.

Сравнение, данное Business Owner: прямой конкурент с более сильными продвижением и поддержкой берет 490 ₽ помесячно и 180 ₽/мес при оплате двух лет. Наша месячная цена ниже на 91 ₽, а годовая не копирует экстремальную скидку за вдвое более длинное обязательство.

## Функциональный Контракт

1. Android получает доступные продукты и цены только от Core.
2. Android отправляет в Core только `product_id`; сумму, срок, аккаунт и провайдера определяет Core.
3. Core создает собственную запись платежа до обращения к провайдеру и использует стабильный UUID как `Idempotence-Key` ЮKassa.
4. ЮKassa получает `capture: true`, `confirmation.type: redirect`, HTTPS `return_url` и неприватный внутренний `payment_id` в metadata.
5. Android открывает полученный `checkout_url` системным браузером. Мобильный SDK и его ключ не используются.
6. Webhook содержит только повод для проверки. Core делает `GET` платежа у ЮKassa и сверяет provider payment ID, статус `succeeded`, `paid`, сумму, валюту и metadata.
7. Успешный платеж в одной транзакции помечается примененным и двигает `vip_expires_at` вперед на снимок оплаченного срока. Повторный webhook возвращает успех без повторного продления.
8. `payment.canceled` завершает платеж без VIP. Неподтвержденный, pending или ошибочный платеж VIP не меняет.
9. По истечении `vip_expires_at` платный VIP возвращается в FREE. Существующий административный VIP без срока остается бессрочным и не меняется этой стадией.
10. Новая покупка доступна только по существующему серверному решению `purchase.Assess`: switch продаж и 7-дневный FREE-период не дублируются и не вычисляются в Android.

## Универсальность

Core определяет интерфейс payment provider: создать checkout и прочитать канонический статус. ЮKassa — один adapter этого интерфейса. Таблицы, публичный Core API, Android и применение entitlement не содержат provider-specific полей, кроме непрозрачных `provider` и `provider_payment_id`. Второй провайдер добавляется новым adapter и одной серверной конфигурацией, не меняя Android, аккаунты, VIP или VPN-доступы.

## Секреты

- GitHub Secrets: `YOOKASSA_TEST_SHOP_ID`, `YOOKASSA_TEST_SECRET_KEY`.
- Workflow передает их только в environment-файл Core как нейтральные runtime-переменные.
- `YOOKASSA_TEST_MOBILE_SDK_KEY` не используется.
- Значения не печатаются, не попадают в Android, репозиторий, документацию, PR, Terraform state или логи.
- API endpoint, payload и provider adapter одинаковы в test и production. Переход требует заменить только shopId и Secret Key.

## Не Входит

- Google Play Billing, мобильный SDK ЮKassa, embedded checkout;
- подписки, автоплатежи, сохранение payment method;
- пользовательский flow продления, напоминания и маркетинг удержания;
- возвраты и chargeback automation;
- production-магазин, договор, фискализация и налоговая настройка;
- изменение существующего решения о видимости VIP-кнопки и правила 7 дней FREE.

## Критерии Приемки

- доступны продукты 1/3/12 месяцев с зафиксированными серверными ценами;
- успешный тестовый платеж проходит Android → Core → ЮKassa redirect → webhook → VIP с правильным сроком;
- отмена и неуспех VIP не выдают;
- возврат в приложение без webhook VIP не выдает;
- повторный webhook и повторная доставка одного платежа не добавляют срок повторно;
- история платежей остается в PostgreSQL;
- Android не содержит YooKassa dependency, shopId, Secret Key или provider-specific API;
- тесты доказывают provider boundary и все перечисленные сценарии;
- PR head проходит required checks, review и не имеет merge conflicts.

## Repository Memory

Обновляются `docs/architecture/entitlement-model.md`, `docs/architecture/secrets-model.md`, `docs/architecture/privacy-model.md`, `docs/architecture/decisions.md`, Android contract и отдельный operational документ интеграции ЮKassa. Не закрытые production-вопросы фиксируются в `docs/tech-debt.md`.

## Legacy Workspace Assessment

Зависимости от отдельного workspace path нет. Ветка создана `scripts/New-FeatureBranch.ps1` от актуального `main`; задача использует штатный branch → PR → checks → review flow.
