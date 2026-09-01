# Spec: Живая приемка платежей и возвратов

- Feature ID: `161-payment-live-acceptance`
- Feature Branch: `feature/161-payment-live-acceptance`
- Status: `live-accepted-with-provider-limitation`

## Goal

Дать Business Owner воспроизводимый и безопасный readback живого test-store контура: локальное состояние payment/refund/VIP сверяется с каноническим server API ЮKassa без вывода email, checkout, provider identifiers, idempotency keys или секретов. Для обязательного частичного refund разрешить ровно одну контролируемую подготовку времени только у успешного тестового платежа.

## Live Evidence

31 августа 2026 года живой полный возврат 399 ₽ прошёл через Android. Core перевёл аккаунт VIP→FREE, отозвал external credential, обе ноды перешли с двух credentials на одну, а сохранённая внешняя ссылка фактически перестала давать интернет. Панель и Device Access доказывают entitlement, но существующие operator workflows не показывают payment/refund row и канонический provider status.

Предыдущая успешная оплата не была обработана webhook и восстановилась кнопкой `Проверить оплату`. Для HTTP Basic Auth ЮKassa требует настраивать webhook в личном кабинете; это остаётся отдельным обязательным live-действием Business Owner.

## Scope

- отдельный GitHub Actions workflow для redacted readback одного аккаунта по безопасному UUID-prefix;
- authenticated GET payment/refund из ЮKassa только внутри CI, с сопоставлением суммы, валюты, test flag и provider object;
- test-only `prepare_partial`: только latest `provider_test=true`, `succeeded`, refundable VIP payment без refund; переводит его policy timestamps в состояние «8 суток после покупки»;
- readback account tier/expiry, payment, logical refund, attempt и provider state;
- tests process-layer и обновление BO/YooKassa/feature-memory после живых прогонов.

## Non-Goals

- не добавлять backdoor или test clock в Core/Android;
- не менять payment/refund policy, цены, каталог или provider adapter;
- не разрешать подготовку времени production payment (`provider_test is not true`);
- не создавать, отменять или возвращать деньги из operator workflow;
- не печатать email, полный account/payment/refund/provider ID, checkout URL, токены или секреты;
- не имитировать недокументированный test-store сценарий `refund insufficient_funds`: если ЮKassa не публикует способ его вызвать, это фиксируется как provider limitation, а не подделывается.

## Repository Memory Updates

- новый task-memory `specs/161-payment-live-acceptance/`;
- live evidence дописывается в `specs/157-yookassa-refunds/` и `specs/160-vip-external-access-revocation/`;
- единый BO status и операционный способ проверки обновляются в `docs/business-owner-operations.md`;
- test-store matrix и ограничения актуализируются в `docs/integrations/yookassa.md` и `docs/tech-debt.md`.

## Legacy Workspace Assessment

Проверены `AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и текущие workflows. Process использует обычную feature branch; зависимости от отдельного workspace path нет.

## Acceptance Criteria

- readback однозначно выбирает один account prefix и отказывает при 0/нескольких совпадениях;
- provider GET выполняется с существующими test secrets, но секреты и полные provider identifiers не попадают в output;
- вывод сопоставляет DB и provider payment/refund status, amount, currency, test flag и payment method;
- `prepare_partial` атомарно меняет только policy timestamps последнего подходящего test payment и account expiry;
- production/non-success/already-refunded/non-VIP payment изменить невозможно;
- live partial refund `bank_card` проходит через Android и возвращает рассчитанную Core сумму на исходный способ;
- full refund evidence фиксирует `399 ₽`, VIP→FREE, external off и node count `2→1`;
- webhook test store настроен для `payment.succeeded`, `payment.canceled`, `refund.succeeded` и проверен новым событием/повтором;
- PR head проходит required checks без blocking findings и конфликтов.

## Validation

- workflow YAML parse и source-contract tests;
- fixture tests redacted provider readback;
- `Process Baseline`, `PR Loop Guard`, `AI Review`;
- live `read → prepare_partial → read → Android refund → read`;
- panel, Device Access и node service log после refund.

## Local Evidence

- fixture unit tests: matching full refund redaction, provider mismatch, ambiguous account, mutation guards — green;
- repository-tool checkout, stray escape и address masking checks — green;
- `git diff --check` — green;
- настоящий PostgreSQL 16 syntax/transaction/production guard выполняется отдельным PR check, потому что локальный Docker daemon недоступен.
