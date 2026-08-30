# Документация Проекта

Проект: MVP Android VPN для российского рынка. Репозиторий содержит process-layer (agent orchestration), целевую архитектуру MVP, инфраструктуру как код и Android-клиент.

`docs/` — durable memory проекта:

**Процесс**

- `docs/ai-pr-workflow.md` — completion contract через PR loop
- `docs/worker-orchestration.md` — локальный branch-based orchestration flow
- `docs/environment.md` — состояние локальной среды и требования к ней
- `docs/stack.md` — зафиксированный стек MVP и его следствия для process-layer

**Архитектура**

- `docs/architecture/` — целевая архитектура MVP: границы компонентов, threat model, privacy model, модель идентификаторов, remote config, bootstrap и восстановление, жизненный цикл нод, observability, entitlement, секреты, инфраструктура, сценарии отказа, инварианты развития, ADR и prerequisites. Карта документов — в `docs/architecture/README.md`

**Интеграции**

- `docs/integrations/brevo.md` — email-провайдер для magic link: секреты, требования к домену отправителя, проверка доставляемости
- `docs/integrations/dns.md` — DNS: провайдеры, роли доменов, конвенции записей, запрет проксирования на точках входа
- `docs/release-apk.md` — официальный APK: release signing, неизменяемая история, `latest` и штатная публикация
- `docs/app-updates.md` — единая latest/min policy, direct APK install и граница будущего Google Play
- `docs/integrations/digitalocean.md` — DigitalOcean: фактическая область токена, Spaces, retention, границы бюджета
- `docs/integrations/libxray.md` — транспортный движок: закреплённая версия, контрольная сумма, публичная сигнатура
- `docs/integrations/yookassa.md` — разовые платежи VIP: provider boundary, webhook, секреты и test-store matrix

Связанный task-memory layer хранится в `specs/<feature-id>/`. Product-code задача не должна стартовать без активной feature-memory папки с `spec.md`, `plan.md` и `tasks.md`.

## Правило Наполнения

В `docs/` попадают только документы, соответствующие текущему состоянию репозитория. `docs/architecture/` описывает принятые решения и ограничения, а не намерения: документ, не имеющий проверяемого следствия, в него не попадает.

Разделы `api/`, `deployment/`, `product/` создаются позже — в момент, когда появляется соответствующий код или развёрнутая инфраструктура, а не заранее.

Что сюда намеренно не кладётся:

- audit / gap-analysis / greenfield-analysis
- transition-документы, описывающие незавершённый переход как факт
- корневые markdown-файлы вне `docs/`, кроме `AGENTS.md` и `CLAUDE.md`
