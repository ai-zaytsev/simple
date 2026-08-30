# Plan: Проверка путей входа с пользовательского устройства

- Feature ID: `155-way-in-device-acceptance`
- Feature Branch: `feature/155-way-in-device-acceptance`
- Owner: `orchestrator`

1. Зафиксировать живые entry и устранить blind spot между Android telemetry, Core whitelist и панелью.
2. Добавить ограниченный фоновый sweep всех bootstrap entries напрямую с устройства.
3. Разделить серверные цели self-probe и разрешённые цели device report.
4. Покрыть контракт Android и Core тестами и обновить durable documentation.
5. Диагностировать неработающий edge и восстановить либо штатно отключить его.
6. Собрать test APK, пройти PR loop и передать APK для проверки на целевом устройстве.
7. После human merge развернуть Core и сверить в панели device checks по каждому включённому entry.

