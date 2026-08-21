# Spec: MVP Architecture Foundation

- Feature ID: `001-architecture-foundation`
- Feature Branch: `feature/001-architecture-foundation`
- Status: `draft`

## Goal

Определить целевую архитектуру MVP Android VPN для российского рынка и границы компонентов до начала основной разработки. Результат стадии — документация, а не продуктовый код.

## Non-Goals

- не писать продуктовый код (Android, Control Plane, node-agent)
- не создавать и не покупать внешние ресурсы: VPS, домены, платные SaaS, аккаунты провайдеров
- не разворачивать инфраструктуру
- не выбирать конкретных коммерческих поставщиков за Business Owner — только готовить решение с рекомендацией
- не вводить Kubernetes, Kafka, Elasticsearch, Redis cluster, service mesh и микросервисную декомпозицию без отдельного обоснования

## Scope

- `docs/architecture/` — целевая архитектура MVP
- `docs/README.md` — обновление durable-индекса
- `specs/001-architecture-foundation/` — task-memory стадии

Продуктовые каталоги (`android/`, `control-plane/`, `node-agent/`, `infra/`) на этой стадии не создаются.

## Обязательные Принципы, Которые Архитектура Должна Соблюдать

Из ТЗ, раздел 2. Каждый принцип должен быть прослеживаемо отражён в документации:

1. Server-managed VPN — клиент исполняет решение сервера, не принимает решений сам.
2. Assume compromise from day one — APK декомпилирован, API изучены, аккаунт противника существует, выданные ему IP известны.
3. Disposable VPN nodes — нода и её IP расходуемы, замена без ручного SSH.
4. Progressive disclosure — клиент никогда не видит fleet, только `primary` + 2 reserve.
5. Privacy-preserving observability — бизнес-метрики есть, browsing history восстановить нельзя.
6. Traffic classification — агрегированный тип нагрузки без определения конкретных сайтов.
7. Разделение идентификаторов — `account_id`, `device_id`, `analytics_id`, `vpn_credential_id`.

## Deliverables

- `overview.md` — architecture diagram и границы Android / Control Plane / VPN nodes / observability
- `threat-model.md` — модель нарушителя, поверхность атаки, контроли, blast radius
- `privacy-model.md` — что собирается, что запрещено, сроки хранения, разделение идентификаторов
- `identity-model.md` — Account / Device / Credential / Analytics identity
- `remote-config.md` — connection plan, remote config, подпись и anti-rollback
- `bootstrap-recovery.md` — как клиент находит Control Plane и восстанавливается без нового APK
- `node-lifecycle.md` — состояния ноды, provisioning, детект блокировки, замена, вывод из эксплуатации
- `observability.md` — data model метрик, traffic classification, пайплайн и хранение
- `entitlement-model.md` — FREE/VIP, квоты, платежи
- `secrets-model.md` — классы секретов, хранение, доставка, ротация
- `infrastructure.md` — инфраструктурная схема MVP и топология
- `failure-scenarios.md` — сценарии отказа, детект, поведение клиента, восстановление
- `decisions.md` — ADR по спорным вопросам
- `prerequisites.md` — внешние ресурсы, которые нужно запросить у Business Owner для следующих стадий

## Acceptance Criteria

- архитектура документирована и внутренне непротиворечива
- у каждого ключевого компонента определена зона ответственности и то, чего он делать не должен
- определены failure scenarios с детектом и путём восстановления
- определён список внешних ресурсов для следующих стадий, без необходимости покупать инфраструктуру заранее
- все семь принципов из раздела 2 ТЗ прослеживаются в документах
- спорные вопросы зафиксированы как ADR со статусом `accepted` либо `needs-owner-decision`

## Validation

- `Process Baseline`, `PR Loop Guard`, `AI Review` зелёные на PR
- перекрёстные ссылки между документами не ведут в несуществующие файлы
- в документации нет решений, требующих покупки ресурсов на этой стадии
