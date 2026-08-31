# Plan: Отзыв внешнего доступа вместе с VIP

- Feature ID: `160-vip-external-access-revocation`
- Feature Branch: `feature/160-vip-external-access-revocation`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать live evidence: FREE + одна app allowance, но две credentials в node sync.
2. Перевести оба operator tier setters на транзакцию и общий FREE downgrade path.
3. Добавить defense-in-depth policy filter в node credential queries.
4. Расширить PostgreSQL lifecycle: manual downgrade, expiration, refund и намеренно stale external row.
5. Обновить durable docs и feature-memory.
6. Пройти PR loop; после human merge развернуть Core.
7. Выполнить полный live refund и проверить tier, external state и node count.

## Risks

- разрыв между сменой tier и отзывом credential позволит подключиться в коротком окне;
- фильтр может ошибочно отрезать app credential FREE-пользователя;
- operator downgrade по email и по prefix могут снова разойтись;
- физическое удаление устройства уничтожит аудит и не требуется для прекращения доступа;
- нода применяет полный список с poll interval, поэтому live readback должен ждать очередную синхронизацию.

## Validation Plan

- transaction tests на настоящем PostgreSQL;
- stale-row test отдельно доказывает node-query defense независимо от clean downgrade;
- exact assertions: external credential `REVOKED`, отсутствует в `LiveCredentials`, app credential остаётся;
- `go test ./...`, `go vet ./...`, `git diff --check`;
- PR loop и post-merge live readback.

## Merge Readiness

Merge-ready требует green checks текущего head SHA, отсутствия blocking findings и конфликтов. Human выполняет merge. Стадия не закрывается до live refund и подтверждения, что ноды перестали получать external credential.
