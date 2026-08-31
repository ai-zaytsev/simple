# Spec: Безопасная повторная доставка webhook ЮKassa

- Feature ID: `162-payment-webhook-replay`
- Feature Branch: `feature/162-payment-webhook-replay`
- Status: `local-ready`

## Goal

Дать Business Owner воспроизводимый live-check повторной доставки уже завершённых
`payment.succeeded` и `refund.succeeded`: Core должен отвечать `200`, повторно
сверять объект с ЮKassa и не выдавать VIP, не продлевать срок и не создавать
второй возврат.

## Non-Goals

- не менять платёжную, refund или VIP business logic;
- не имитировать новый успех ЮKassa и не подменять канонический provider status;
- не публиковать provider object ID, секреты, email или полный account ID;
- не считать live-проверкой сценарий `payment.canceled`, пока ЮKassa реально не
  переведёт отдельный платёж в этот финальный статус.

## Scope

- отдельный manual GitHub workflow для строго test-store payment/refund;
- redacted Python validator/replayer и его unit tests;
- baseline check, BO/runbook и YooKassa integration memory;
- runtime Core, Android, PostgreSQL schema и provider adapter не меняются.

## Safety Contract

- account prefix должен однозначно указывать ровно на один аккаунт;
- разрешён только последний `yookassa`, `provider_test=true`, `succeeded`
  payment с уже успешно завершённым refund;
- webhook body строится внутри CI из private snapshot, полные provider ID не
  печатаются и не становятся workflow input/output;
- до и после четырёх повторов сравниваются tier, VIP expiry, entitlement
  timestamps, refund count и refund amount;
- любой `applied=true`, non-200, изменение snapshot или неоднозначность
  останавливают workflow с ошибкой.

## Repository Memory Updates

- обновить `docs/integrations/yookassa.md`, `docs/business-owner-operations.md`
  и `docs/tech-debt.md` после live readback;
- поддерживать этот `spec.md`, `plan.md` и `tasks.md` до merge-ready и live
  acceptance.

## Legacy Workspace Assessment

Зависимости от альтернативной workspace-механики нет. Проверено по
`AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и текущим workflow:
задача выполняется как `feature branch -> PR -> checks -> human merge`.

## Acceptance Criteria

- workflow повторно доставляет два `payment.succeeded` и два
  `refund.succeeded` для одной завершённой test-store цепочки;
- каждый вызов получает HTTP `200`, `received=true`, `applied=false`;
- Core остаётся `FREE`, VIP expiry не появляется, entitlement timestamps не
  меняются, refund остаётся один и на исходную сумму;
- публичный лог содержит только внутренние восьмисимвольные prefixes и safe
  verdict;
- PR head проходит required checks, AI review и не имеет conflicts.

## Validation

- `python3 .github/scripts/test_payment_webhook_replay.py`;
- `python3 .github/scripts/test_payment_acceptance.py`;
- YAML/contract review и `Process Baseline`, `PR Loop Guard`, `AI Review`;
- после merge: live workflow и `Read The Panel` с явно названными числами.
