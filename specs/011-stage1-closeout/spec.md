# Spec: Stage 1 Closeout

- Feature ID: `011-stage1-closeout`
- Feature Branch: `feature/011-stage1-closeout`
- Status: `implemented`

## Goal

Закрыть Стадию 1 ТЗ — архитектурный фундамент — и зафиксировать, что из требований выполнено, а что остаётся открытым перед следующей стадией.

## Проверка Соответствия Стадии 1

ТЗ требовало подготовить четырнадцать артефактов. Все существуют:

| Требование ТЗ | Где |
| --- | --- |
| Общая architecture diagram | `docs/architecture/overview.md` |
| Границы Android / Control Plane / VPN nodes / observability | `docs/architecture/overview.md` |
| Threat model | `docs/architecture/threat-model.md` |
| Privacy model | `docs/architecture/privacy-model.md` |
| Модель Account / Device / Credential | `docs/architecture/identity-model.md` |
| Remote config model | `docs/architecture/remote-config.md` |
| Bootstrap / recovery model | `docs/architecture/bootstrap-recovery.md` |
| Node lifecycle | `docs/architecture/node-lifecycle.md` |
| Observability data model | `docs/architecture/observability.md` |
| Entitlement / payment model | `docs/architecture/entitlement-model.md` |
| Secrets model | `docs/architecture/secrets-model.md` |
| Инфраструктурная схема MVP | `docs/architecture/infrastructure.md`, `docs/architecture/mvp-topology.md` |
| Список инфраструктурных prerequisites | `docs/architecture/prerequisites.md` |
| Архитектурные решения по спорным вопросам | `docs/architecture/decisions.md`, ADR-001 – ADR-019 |

Критерии завершения стадии:

- архитектура документирована: да
- у каждого компонента понятная ответственность и явный список того, чего он делать не должен: да, `overview.md`
- определены failure scenarios: да, `failure-scenarios.md`, семнадцать сценариев с детектом и восстановлением
- определено, какие внешние ресурсы нужны на следующих стадиях: да, `prerequisites.md`
- нет необходимости покупать всю инфраструктуру заранее: да, плановая конфигурация $31/мес при лимите $45, ничего не создано авансом

## Что Сделано Сверх Стадии 1

Prerequisites не только перечислены, но и получены и проверены на реальных API:

- email-провайдер Brevo: домен аутентифицирован, письма доставлены на два ящика
- DNS: два провайдера, четыре entry-домена и домен сайта инвентаризированы
- DigitalOcean: область токена установлена, права записи подтверждены
- Spaces: доступ проверен round-trip, границы прав ключей установлены
- Terraform: скелет с remote state в Spaces, бюджетный guard, `init` проходит
- SSH-ключ `simple-vpn-ssh-key` зарегистрирован и подтверждён

## Что Остаётся Открытым

Ни один пункт не блокирует переход к следующей стадии, но каждый блокирует что-то дальше:

| Вопрос | Блокирует | Срок |
| --- | --- | --- |
| DNS-резолвер, `ADR-008` | Профиль DNS в плане | До развёртывания нод |
| Резервный email-провайдер, `ADR-012` | Устойчивость входа новых пользователей | До публичного запуска |
| Площадки для `rescue`-зеркал, `ADR-004` | Первую установку при массовой блокировке | До публичного запуска |
| Второй провайдер для нод, `ADR-005` | Независимость флота от одного провайдера | До публичного запуска |
| Зона `simple-vpn.download` | Управление почтовыми записями автоматизацией | Когда понадобится срочная правка |
| Проверка шейпинга 20 Мбит/с | Требование лимита скорости | Развёртывание первой VPN-ноды |

## Acceptance Criteria

- соответствие Стадии 1 зафиксировано пунктом к пункту
- открытые вопросы перечислены с указанием, что именно каждый блокирует
- `ssh_key_names` заполнен подтверждённым ключом
