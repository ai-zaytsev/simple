# Spec: Отзыв внешнего доступа вместе с VIP

- Feature ID: `160-vip-external-access-revocation`
- Feature Branch: `feature/160-vip-external-access-revocation`
- Status: `live-accepted`

## Goal

Любая внешняя VPN-ссылка должна переставать работать, как только аккаунт больше не имеет тарифа, разрешающего внешние устройства. Смена тарифа и отзыв credentials происходят атомарно, а выдача credentials нодам дополнительно проверяет текущий tier limit.

## Live Finding

31 августа 2026 года после payment deploy панель показывала один FREE-аккаунт и нулевой VIP, а обе VPN-ноды продолжали получать две credentials. Для FREE разрешена одна app credential и ноль external; вторая credential принадлежала ранее созданному внешнему устройству. Источник рассинхронизации — operator path `SetAccountTierByPrefix`: он менял tier на FREE, но не применял access limits. `LiveCredentials` доверял флагу credential `ACTIVE` и не проверял `max_external` текущего тарифа.

## Non-Goals

- не удалять историю устройств физически из PostgreSQL;
- не отзывать единственную разрешённую app credential FREE-пользователя;
- не менять правила FREE/VIP, цены, оплату или refund policy;
- не отключать действующий оплаченный VIP до успешного refund/истечения/явной operator-команды.

## Scope

- оба operator tier paths: email и безопасный account-prefix;
- существующий общий downgrade path `expireAccount`;
- node `LiveCredentials` и согласованный список limited credentials;
- PostgreSQL lifecycle tests для manual downgrade, expiry, refund и stale-row defense;
- BO/integration documentation и feature-memory.

## Repository Memory Updates

- обновить `docs/business-owner-operations.md` и профильную техническую документацию о последствиях VIP→FREE;
- записать live finding и проверки в `specs/160-vip-external-access-revocation/`;
- после merge продолжить live refund matrix задачи `157-yookassa-refunds`.

## Legacy Workspace Assessment

Репозиторий использует branch-based flow; отдельной workspace-зависимости в store, node sync, workflows или feature process нет. Работа начата от merged `main` после чтения repository contracts.

## Acceptance Criteria

- operator VIP→FREE отзывает все active external credentials в одной транзакции со сменой tier;
- expiry и confirmed refund сохраняют ту же гарантию;
- FREE account с ошибочно оставшейся active external row не передаёт её VPN-нодам;
- app credential в пределах FREE limit продолжает работать;
- после отзыва устройство может остаться как историческая запись, но его link не выдаётся и credential не принимается;
- PostgreSQL lifecycle проверяет реальные запросы, состояния credentials и node list;
- PR head проходит required checks без blocking findings/конфликтов;
- после merge live refund переводит аккаунт в FREE, а node user count уменьшается с двух до одной credential.

## Validation

- `go test ./...`, `go vet ./...`;
- PostgreSQL 16 lifecycle workflow;
- PR required checks;
- post-deploy panel/service log;
- live full refund и node credential readback после poll interval.

## Local Evidence

- `go test -count=1 ./...` — green;
- `go vet ./...` — green;
- `git diff --check` — green;
- настоящие PostgreSQL lifecycle tests требуют `TEST_DATABASE_URL` и выполняются в PR workflow на PostgreSQL 16.

## PR Evidence

- PR: `#166`;
- checked head: `0757c718bfcacab8add5e30ee58a5ca31e194687`;
- `AI Review`, `PR Loop Guard`, `PostgreSQL Lifecycle`, `Process Baseline` — green;
- mergeable state — clean, blocking findings отсутствуют.

## Live Evidence

31 августа 2026 года merged Core развернут с `main`. Перед full refund обе ноды стабильно получали две credentials; после канонического refund account стал FREE, external credential — неактивным, а обе ноды на ближайшем poll перешли к одной app credential. Business Owner отдельно подтвердил, что сохранённая VIP-ссылка перестала давать интернет. Purchases setting после теста возвращён с 1 на 7 дней.
