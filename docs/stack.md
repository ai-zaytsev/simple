# MVP Stack

Зафиксированное решение по стеку MVP. Документ — durable memory: он описывает выбор технологий, а не архитектуру приложения. Проектирование приложения на момент фиксации не начато.

## Состав

| Слой | Технологии |
| --- | --- |
| Android | Kotlin + Jetpack Compose |
| VPN на Android | Android `VpnService` |
| VPN engine | Xray-core / libXray |
| Протокол | VLESS + REALITY |
| Backend / Control Plane | Go |
| Основная БД | PostgreSQL |
| Per-user analytics | ClickHouse |
| Metrics | Prometheus |
| Dashboards | Grafana |
| Logs | Loki |
| Telemetry transport | OpenTelemetry |
| Provisioning VPS | Terraform + cloud-init + API провайдера |
| Management network | WireGuard |
| Email login | Transactional email provider + magic links |
| APK testing | Firebase App Distribution + signed APK |
| CI/CD | GitHub Actions |
| Официальный APK-сайт | Nginx на DigitalOcean `site-1`, Cloudflare, APK в Spaces |
| VPN nodes | Ubuntu 24.04 + Xray + node-agent |

## Что Из Этого Следует Для Process-Layer

- репозиторий будет multi-language (Go, Kotlin, HCL), поэтому `Baseline Checks` проверяет наличие тулчейна адаптивно и пропускает шаг, пока соответствующего кода нет
- `.gitignore` покрывает артефакты Go, Gradle/Android и Terraform, а также локальные секреты
- продуктовый CI (сборка APK, тесты Go, `terraform plan`) не заводится авансом: он появляется вместе с первой задачей, которая приносит соответствующий код
- signing keys, provider API tokens, SMTP-креды и Firebase service account никогда не попадают в репозиторий — только в GitHub Secrets или локальный `.env`

## Что Ещё Не Решено

Эти вопросы намеренно оставлены открытыми и должны решаться в рамках отдельных задач через `specs/<feature-id>/`:

- раскладка каталогов репозитория (single repo vs отдельные модули)
- конкретный cloud-провайдер для VPS и его Terraform provider
- конкретный transactional email provider
- модель хранения состояния Control Plane и схема PostgreSQL
- контракт между Control Plane и node-agent

Пока решение не принято и не зафиксировано в `docs/`, реализация по нему не стартует.
