# Технологический стек

Документ фиксирует используемые технологии, а не размещение и не текущее состояние сервисов. Актуальные провайдеры, регионы, хосты и операции находятся только в [BO-инструкции](business-owner-operations.md).

## Состав

| Слой | Технологии |
| --- | --- |
| Android | Kotlin + Jetpack Compose |
| VPN на Android | Android `VpnService` |
| VPN engine | Xray-core / libXray |
| Основной VPN-транспорт | VLESS over WebSocket/TLS за Nginx |
| Запасной транспорт | REALITY на отдельном порту; включается только подписанным планом |
| Backend / Control Plane | Go |
| Основная БД | PostgreSQL |
| Аналитика и метрики MVP | PostgreSQL + серверная `/panel` |
| Логи | journald на хостах |
| Provisioning VPS | Terraform + cloud-init + API провайдера |
| Доступ к VPN-нодам | SSH только с Core; GitHub Actions подключается через Core как jump host |
| Email login | Brevo + magic links |
| APK testing | Firebase App Distribution + signed APK |
| CI/CD | GitHub Actions |
| Официальный APK-сайт | Nginx на DigitalOcean `site-1`, Cloudflare, APK в Spaces |
| VPN nodes | Kamatera, Ubuntu + Nginx + Xray + node-agent |

ClickHouse, Prometheus, Grafana, Loki, OpenTelemetry Collector и отдельная WireGuard management network сейчас не развёрнуты. Это возможные будущие компоненты с условиями ввода из [deferred-stack-migration.md](architecture/deferred-stack-migration.md), а не часть действующего стека.

## Что Из Этого Следует Для Process-Layer

- репозиторий будет multi-language (Go, Kotlin, HCL), поэтому `Baseline Checks` проверяет наличие тулчейна адаптивно и пропускает шаг, пока соответствующего кода нет
- `.gitignore` покрывает артефакты Go, Gradle/Android и Terraform, а также локальные секреты
- продуктовый CI (сборка APK, тесты Go, `terraform plan`) не заводится авансом: он появляется вместе с первой задачей, которая приносит соответствующий код
- signing keys, provider API tokens, SMTP-креды и Firebase service account никогда не попадают в репозиторий — только в GitHub Secrets или локальный `.env`

Открытые продуктовые и операционные решения ведутся в [tech-debt.md](tech-debt.md), [release-blockers.md](release-blockers.md) и активной feature-memory, а не в этом перечне технологий.
