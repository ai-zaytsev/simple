# Plan: MVP Architecture Foundation

- Feature ID: `001-architecture-foundation`
- Feature Branch: `feature/001-architecture-foundation`
- Owner: `orchestrator`

## Implementation Slices

1. Каркас `docs/architecture/` и общий обзор: диаграмма, границы компонентов, зоны ответственности.
2. Security-слой: threat model, privacy model, identity model, secrets model.
3. Control-слой: remote config, bootstrap/recovery, node lifecycle, entitlement.
4. Data-слой: observability data model и traffic classification.
5. Инфраструктура: топология MVP, prerequisites для следующих стадий.
6. Сведение: failure scenarios, ADR по спорным вопросам, обновление `docs/README.md`.

Порядок неслучаен: threat model и privacy model задают ограничения, которым обязаны подчиняться remote config, observability и node lifecycle. Обратный порядок привёл бы к архитектуре, которую пришлось бы переписывать под ограничения.

## Ключевые Архитектурные Развилки

Каждая развилка должна закрыться в `decisions.md` либо решением, либо явным статусом `needs-owner-decision`:

- как клиент находит Control Plane, если основной домен заблокирован
- как ограничить blast radius при том, что противник может создать аккаунт и увидеть выданные ему IP
- как считать per-user аналитику, не имея возможности восстановить browsing history
- где проходит граница между тем, что нода видит транзитно, и тем, что она сохраняет
- как разделить FREE и VIP так, чтобы разведка через FREE-аккаунт не раскрывала VIP-мощности
- чем принимать платежи в РФ, если Google Play Billing недоступен
- какой провайдер VPS и какой email-провайдер — решение Business Owner, не агента

## Risks

- архитектура «на бумаге», не проверяемая на следующих стадиях — снимается тем, что каждый документ фиксирует наблюдаемые критерии, а не намерения
- privacy-требования и observability-требования конфликтуют — снимается явной моделью данных с указанием сроков хранения и точки агрегации
- соблазн спроектировать инфраструктуру под масштаб, которого у MVP нет — снимается запретом на Kubernetes, Kafka, Elasticsearch, Redis cluster и service mesh без обоснования
- соблазн закупить ресурсы заранее — снимается вынесением всех внешних ресурсов в `prerequisites.md`
- security-by-obscurity как скрытая опора архитектуры — снимается threat model, где неизвестность IP, домена и REALITY profile явно не считается security boundary

## Validation Plan

- проверка, что все семь принципов раздела 2 ТЗ прослеживаются в документах
- проверка, что ни один документ не требует покупки ресурсов на этой стадии
- проверка перекрёстных ссылок между документами
- PR loop: `Process Baseline`, `PR Loop Guard`, `AI Review`

## Merge Readiness

Стадия готова, когда PR loop зелёный, спорные вопросы закрыты или явно вынесены Business Owner, а список prerequisites передан. Merge остаётся ручным действием человека.
