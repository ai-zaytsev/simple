# Spec: Каноническая перепроверка платежа

- Feature ID: `159-payment-reconciliation`
- Feature Branch: `feature/159-payment-reconciliation`
- Status: `merge-ready`

## Goal

Исправить найденный в live test-store дефект: кнопка Android `Проверить оплату` должна просить Core канонически перечитать незавершённый платёж у исходного провайдера. Подтверждённый ЮKassa платёж активирует VIP ровно один раз даже при потерянном или задержанном webhook; pending/canceled, ошибка провайдера и подмена полей VIP не активируют.

## Live Finding

31 августа 2026 года тестовая карточная оплата вернулась в Android, но осталась в состоянии `pending`. Повторное нажатие `Проверить оплату` ничего не меняло. Серверный журнал не содержал обработки payment webhook. Причина в контракте `GET /v1/payments/current`: он только читал PostgreSQL и принципиально не обращался к провайдеру, хотя UI обещал проверку.

## Non-Goals

- не доверять return URL или полям Android как подтверждению оплаты;
- не включать VIP вручную и не ослаблять проверку amount/currency/metadata/paid;
- не заменять и не отключать webhook: он остаётся основным push-механизмом;
- не менять каталог, цены, сроки VIP и refund policy;
- не добавлять polling в фоне Android.

## Scope

- provider-neutral payment service: reconcile текущего pending-платежа по его сохранённому provider ID;
- `GET /v1/payments/current`: каноническая проверка только для незавершённого платежа;
- unit/API tests на success, cancel, mismatch, provider outage и exactly-once;
- Android-контракт не меняется: существующая кнопка повторяет тот же endpoint;
- durable YooKassa/операционная документация и feature-memory.

## Repository Memory Updates

- обновить `docs/integrations/yookassa.md`, описав recovery при потерянном webhook;
- обновить `specs/159-payment-reconciliation/{spec,plan,tasks}.md` по фактическому состоянию;
- после merge вернуться к live matrix из `specs/157-yookassa-refunds/`.

## Legacy Workspace Assessment

Репозиторий использует branch-based flow. Проверены `AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и текущее состояние веток; отдельный workspace path для реализации не нужен.

## Acceptance Criteria

- pending payment с каноническим `succeeded + paid` после нажатия `Проверить оплату` становится succeeded и активирует VIP;
- сумма, валюта, metadata и provider payment ID сверяются так же строго, как в webhook path;
- повторная проверка подтверждённого платежа не продлевает VIP повторно;
- pending/canceled и ошибка провайдера не активируют VIP;
- неизвестный или не полностью созданный платёж не вызывает запрос с пустым provider ID;
- webhook продолжает работать и использует тот же единый reconcile path;
- PR head проходит required checks без blocking findings и конфликтов;
- после merge Core/APK развёрнуты, live платёж прочитан повторно, панель сверена.

## Validation

- `go test ./...` и `go vet ./...` в `control-plane`;
- Android unit tests/assemble через существующий CI, если Android-файлы не меняются — подтверждение отсутствия contract change;
- PR checks: `Process Baseline`, `PR Loop Guard`, `AI Review` и затронутые product checks;
- post-deploy public route smoke, service log и panel readback;
- live test-store: существующий pending платёж после кнопки становится каноническим provider status без ручного VIP.
