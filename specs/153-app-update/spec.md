# Spec: Обновление приложения

- Feature ID: `153-app-update`
- Feature Branch: `feature/153-app-update`
- Status: `active`

## Goal

Дать Android-приложению единый серверный контракт обновлений, который независимо от канала распространения сообщает последнюю и минимально поддерживаемую версии. Для прямой установки приложение скачивает APK по выданной Core HTTPS-ссылке, сверяет SHA-256 и передаёт проверенный файл стандартному установщику Android. Версия ниже минимальной не получает новый connection plan и не может запустить VPN.

## Product Decisions

- Сравнивается целочисленный Android `versionCode`, а `versionName` остаётся только подписью для человека.
- Обязательность не хранится отдельным противоречивым флагом: Core задаёт `min_supported_version`, а verdict однозначен — `current < min` означает принудительное обновление и запрет VPN.
- `current < latest` при `current >= min` означает добровольное обновление с действиями «Обновить / Позже» и без ограничения VPN.
- Общий policy/verdict не знает, как устанавливается сборка. Реализация канала `direct_apk` получает URL и SHA-256; будущая реализация `google_play` сможет использовать flexible/immediate flow с теми же `latest/min`.
- Подписанный remote config версии `v=1` расширяется совместимыми полями. Корневой `min_supported_app_version` сохраняется для уже выпущенных клиентов.
- Core повторно проверяет `app_version` в запросе connection plan до выдачи плана или credential. Клиентская проверка отвечает за UX, серверная — за фактический запрет запуска VPN.
- Публикация официального APK после публичной проверки автоматически синхронизирует `latest` и артефакт с Core; повышение `min` выполняется отдельной штатной operator-операцией и никогда не происходит как побочный эффект публикации.

## Scope

- `control-plane/internal/document`, `store`, `api` и миграция серверной update policy.
- `GET /v1/config`, проверка версии в `POST /v1/plan`, закрытые admin endpoint'ы и operator workflow.
- `Publish APK`: передача в Core уже опубликованных versionCode/versionName/public URL/SHA-256.
- Android: единый update verdict, обновление на `BuildConfig.VERSION_CODE`, UI добровольного/принудительного обновления, direct APK download/hash/install flow, обработка ошибок.
- Тесты Core, store, Android decision logic и source/contract tests.
- Durable architecture/operations docs, панель/readback и feature-memory.

## Non-Goals

- Публикация в Google Play и подключение Play Core library сейчас.
- Автоматическое фоновое обновление без решения пользователя.
- Собственный установщик, обход системного подтверждения или хранение APK вне приватного каталога приложения.
- Differential APK, rollback на меньший `versionCode`, управление Android signing key.
- Снятие существующих общих release blockers ради публикации APK этой стадии.

## Security And Failure Contract

- Update policy приходит только внутри существующего подписанного документа; неподписанный URL или hash не используется.
- Разрешён только HTTPS URL и SHA-256 из 64 hex-символов.
- APK пишется в приватный cache-каталог, проверяется до создания install session и удаляется при несовпадении hash.
- APK передаётся через platform `PackageInstaller.Session`; файл остаётся в private cache и storage permissions приложению не нужны.
- Недоступность Core не превращается в глобальный kill switch: применяется последний подписанный config. При попытке получить plan устаревшая версия всё равно получает серверный отказ.
- При недоступном APK, неверном hash, запрете unknown source или отказе установщика приложение остаётся на старой версии и показывает повторяемую ошибку. Принудительный verdict при этом не снимается.
- `latest` не может уменьшаться; `min` всегда находится в диапазоне `1..latest` и может быть понижен оператором для аварийного восстановления поддержки старой сборки.

## Repository Memory Updates

- Обновить `docs/architecture/remote-config.md`, `android-client.md`, `overview.md`, `decisions.md` и architecture index.
- Добавить операционный документ управления версиями и обновить `docs/release-apk.md`.
- Записать невозможность live APK acceptance до закрытия общих release blockers в `docs/tech-debt.md`, если блокеры останутся открыты к merge-ready.
- Поддерживать `spec.md`, `plan.md`, `tasks.md` в `specs/153-app-update/` по фактическому состоянию.

## Legacy Workspace Assessment

Проверены `AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и scripts process-layer. Задача не зависит от альтернативного workspace path: используется один branch-based путь `feature/153-app-update -> PR -> checks -> AI review -> human merge`.

## Acceptance Criteria

- При `current == latest` update UI не показывается, VPN работает.
- При `min <= current < latest` показаны «Обновить / Позже»; отказ оставляет VPN доступным.
- При `current < min` диалог нельзя закрыть, VPN не запускается, а Core не выдаёт connection plan.
- Для `direct_apk` Core передаёт HTTPS URL и SHA-256; клиент скачивает, проверяет hash и передаёт APK системному Android installer.
- Неверный hash никогда не доходит до installer; недоступность APK и ошибка installer видимы и допускают повтор.
- После установки сборка с новым `versionCode` получает обычный verdict и VPN снова запускается.
- Публикация новой официальной версии штатно продвигает Core latest без ручного редактирования сайта или приложения.
- Повышение/чтение min/latest выполняется с сервера без APK update и видно в post-deploy readback.
- Добавление Google Play требует новой channel implementation, но не изменения Core version policy, update verdict, аккаунтов, VIP или VPN entitlement.
- Feature branch проходит required checks и AI review без blocking findings; merge authority остаётся у человека.

## Validation

- `go test -count=1 ./...` и `go vet ./...` в `control-plane`.
- Чистый PostgreSQL migration/read/write/plan-denial integration test.
- Android `testDebugUnitTest` и `assembleDebug`/CI Android Build.
- Тесты no update, optional, later, required, missing artifact, invalid hash, unavailable APK, installer failure и recovery contract.
- `actionlint` для изменённых workflows и проверка отсутствия секретов/небезопасных URL в APK contract.
- PR Loop Guard, Process Baseline, Android Build и AI Review на текущем PR head SHA.
- После deploy — `Read The Panel` и operator readback; после снятия release blockers — подписанный APK end-to-end acceptance.
