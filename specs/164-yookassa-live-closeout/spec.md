# Spec: Закрытие живой приемки ЮKassa

- Feature ID: `164-yookassa-live-closeout`
- Feature Branch: `feature/164-yookassa-live-closeout`
- Status: `live-accepted`

## Goal

Собрать в repository memory окончательные живые доказательства test-store
оплаты/возвратов, закрыть связанные feature tasks и вернуть временные настройки
покупки к продуктовым значениям.

## Scope

- зафиксировать success, failed card attempt, user cancel, webhook/repeat, full
  и partial refunds, lost-response retry, entitlement/access readbacks;
- отметить `open=true`, `FREE=7 дней` и `FREE=1/VIP=0` после тестов;
- исправить одну неточную нейтральную подпись общего durable verifier;
- явно оставить provider limitation для refund insufficient balance.

## Non-Goals

- не менять payment/refund/VIP business logic;
- не имитировать отсутствующий test-store trigger ЮKassa;
- не включать production магазин, чеки, подписки или автоплатежи.

## Repository Memory And Workspace

BO-инструкция остаётся источником операционной истины, integration doc —
технического provider contract, tech debt — незакрытого ограничения. Работа идёт
обычной feature branch от merged main; альтернативной workspace зависимости нет.

## Acceptance Criteria

- документы не противоречат живым run IDs и числам;
- задачи 157/161/162/163 отражают фактическое завершение;
- generic durable verdict не называет любой retry webhook;
- required checks зелёные, conflicts/findings отсутствуют;
- после merge финальная панель по-прежнему показывает продажи открыты, FREE 7,
  FREE=1, VIP=0.

## Validation

- payment/refund Python tests;
- `git diff --check`, docs consistency search;
- Process Baseline, PR Loop Guard, AI Review.
