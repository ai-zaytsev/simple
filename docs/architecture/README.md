# Architecture

Целевая архитектура MVP Android VPN. Стадия 1 — архитектурный фундамент: документы определяют границы компонентов и ограничения, которым обязана подчиняться реализация. Продуктового кода на этой стадии нет.

## Карта Документов

| Документ | О чём | Читать, если |
| --- | --- | --- |
| [overview.md](overview.md) | Диаграмма системы, границы и зоны ответственности компонентов | Нужна общая картина. Начинать отсюда |
| [threat-model.md](threat-model.md) | Модель нарушителя, контроли, blast radius, принятые риски | Проектируете что-либо, доступное извне |
| [privacy-model.md](privacy-model.md) | Что собирается, что запрещено, сроки хранения | Трогаете данные, логи или метрики |
| [identity-model.md](identity-model.md) | `account_id`, `device_id`, `analytics_id`, `vpn_credential_id` и матрица видимости | Добавляете поле с пользовательским ключом |
| [remote-config.md](remote-config.md) | Connection plan, remote config, подпись, anti-rollback | Работаете с конфигурацией клиента |
| [bootstrap-recovery.md](bootstrap-recovery.md) | Как клиент находит Control Plane и восстанавливается без новой сборки | Занимаетесь точками входа и устойчивостью |
| [node-lifecycle.md](node-lifecycle.md) | Состояния ноды, provisioning, детект блокировки, замена | Работаете с флотом |
| [observability.md](observability.md) | Модель данных метрик и классификация нагрузки | Добавляете метрику или дашборд |
| [entitlement-model.md](entitlement-model.md) | FREE и VIP, квоты, платежи | Трогаете доступ или монетизацию |
| [secrets-model.md](secrets-model.md) | Реестр секретов, хранение, ротация, реакция на утечку | Появился новый секрет |
| [infrastructure.md](infrastructure.md) | Топология MVP, сетевые правила, окружения, бэкапы | Разворачиваете что-либо |
| [mvp-topology.md](mvp-topology.md) | Фактическое размещение ролей, стоимость, бюджет трафика | Считаете деньги или добавляете сервер |
| [failure-scenarios.md](failure-scenarios.md) | Сценарии отказа с детектом и восстановлением | Проектируете поведение при ошибке |
| [evolution.md](evolution.md) | Инварианты, обеспечивающие резервирование Core и второй транспорт в будущем | Пишете код Control Plane или клиента |
| [deferred-stack-migration.md](deferred-stack-migration.md) | Ограничения формата, без которых переход на ClickHouse и Loki потеряет историю | Проектируете схему аналитики или формат логов |
| [decisions.md](decisions.md) | ADR по спорным вопросам | Кажется, что решение можно принять иначе |
| [prerequisites.md](prerequisites.md) | Внешние ресурсы, которые нужно запросить у Business Owner | Планируете следующую стадию |

## Прослеживаемость Принципов ТЗ

Каждый обязательный принцип раздела 2 ТЗ и место, где он зафиксирован как проверяемое ограничение:

| Принцип ТЗ | Где реализован |
| --- | --- |
| 2.1 Server-managed VPN | [remote-config.md](remote-config.md), границы клиента в [overview.md](overview.md), `ADR-001` |
| 2.2 Assume compromise from day one | [threat-model.md](threat-model.md) целиком, [secrets-model.md](secrets-model.md) правило «в APK нет секретов» |
| 2.3 Disposable VPN nodes | [node-lifecycle.md](node-lifecycle.md), provisioning в [infrastructure.md](infrastructure.md) |
| 2.4 Progressive disclosure | Схема плана в [remote-config.md](remote-config.md), контроли перечисления в [threat-model.md](threat-model.md), `ADR-002` |
| 2.5 Privacy-preserving observability | [privacy-model.md](privacy-model.md), каталог метрик в [observability.md](observability.md) |
| 2.6 Traffic classification | Раздел классификации в [observability.md](observability.md), `ADR-009` |
| 2.7 Разделение идентификаторов | [identity-model.md](identity-model.md), матрица видимости, `ADR-003` |

## Решения Business Owner, Учтённые В Архитектуре

| Решение | Что изменилось |
| --- | --- |
| Достаточно агрегированного retention | Механизм когорт в [identity-model.md](identity-model.md); ADR-003 закрыт |
| Резервный способ первой настройки обязателен до публичного запуска | Канал `rescue` спроектирован и обязателен: зеркала и код восстановления, [bootstrap-recovery.md](bootstrap-recovery.md); ADR-004 закрыт |
| Один Core и один транспорт — временные ограничения | Шестнадцать инвариантов в [evolution.md](evolution.md); ADR-011 и ADR-014 переведены в `accepted (temporary)` |
| Оплата откладывается | Платёжный путь убран из MVP, VIP выдаётся административно; ADR-006 |
| FREE и VIP — одинаково расходуемая инфраструктура, различие только в лимитах и экспорте | Разделение пулов FREE/VIP убрано из [threat-model.md](threat-model.md) и [node-lifecycle.md](node-lifecycle.md); ADR-002 пересмотрен |
| VIP может выгружать конфигурацию для сторонних клиентов | Subscription-модель и `export`-пул, [entitlement-model.md](entitlement-model.md); ADR-017 |

## Открытые Вопросы

Осталось одно решение со статусом `needs-owner-decision` в [decisions.md](decisions.md): DNS-резолвер (`ADR-008`). Email-провайдер — Brevo (`ADR-012`), VPS-провайдер — DigitalOcean (`ADR-005`).

Отдельно ждут решения Business Owner, не будучи ADR: размер квоты FREE (бюджетное решение, см. [mvp-topology.md](mvp-topology.md)), резервный email-провайдер, площадки для `rescue`-зеркал, второй провайдер для нод. Полный список — в [prerequisites.md](prerequisites.md).

## Как Пользоваться Этими Документами

Документы задают ограничения, а не рекомендации. Если реализация расходится с документом, расхождение устраняется либо в коде, либо новым ADR — но не молча.
