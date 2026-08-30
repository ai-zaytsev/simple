# Plan: Обновление приложения

- Feature ID: `153-app-update`
- Feature Branch: `feature/153-app-update`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать channel-neutral policy: versionCode, derived optional/required verdict и backward-compatible signed config.
2. Добавить атомарное серверное хранение latest/min/channel artifact, admin read/write и panel readback.
3. Запретить выдачу connection plan версии ниже min до создания новых VPN-данных.
4. Подключить официальный APK publisher к обновлению Core latest; добавить отдельный operator workflow для min.
5. Убрать Android-константы версии в пользу `BuildConfig.VERSION_CODE`, добавить общий update controller/verdict.
6. Реализовать direct APK channel: HTTPS download в private cache, SHA-256, доверие unknown source, system PackageInstaller.
7. Добавить optional/required UI и восстановление обычной работы после обновления.
8. Покрыть server/store/Android/workflow contracts тестами и обновить durable docs.
9. Пройти локальную валидацию, PR loop и после human merge выполнить deploy/readback.

## Compatibility

- Старые APK продолжат читать корневой `min_supported_app_version` и игнорируют новые поля JSON.
- Новые APK используют один policy для всех каналов; `direct_apk` выбирается build-константой, а не серверной догадкой.
- `POST /v1/plan` уже передаёт `app_version`; меняется только серверный verdict.
- Повышение latest не повышает min и потому не превращает добровольное обновление в принудительное.

## Risks

- Несогласованные latest/min могут заблокировать весь парк — предотвращается одной транзакцией, monotonic latest и `1 <= min <= latest`; min можно намеренно понизить для аварийного восстановления.
- Продвижение Core до доступности APK оставит обязательный экран без файла — publish workflow обновляет Core только после публичной hash/signature проверки.
- Подмена APK — signed config + SHA-256 до install session + штатная проверка package/signature Android.
- Старый config при сетевой ошибке — сохраняется существующая anti-rollback sequence logic; сервер отдельно отказывает устаревшему plan request.
- Android installer отличается по версиям ОС — используется platform `PackageInstaller`, пользовательское подтверждение и явная обработка status/error.
- Общие release blockers не позволяют сейчас опубликовать acceptance APK — код и deploy/readback проходят отдельно, незавершённый live сценарий фиксируется durable debt.

## Validation Plan

- Unit: policy verdict boundary, parser compatibility, hash validation, Core plan denial/status codes, admin monotonic rules.
- Integration: миграция и update policy lifecycle на чистой PostgreSQL.
- Android CI: unit tests, formatting resources, debug build.
- Workflow: `actionlint`, publish ordering/source assertions.
- PR: required checks, current-head AI review, no conflicts.
- Live: deploy, panel/operator readback; direct APK matrix после разрешения публикации.

## Merge Readiness

Текущий PR head SHA имеет green required checks и Android Build, не имеет blocking findings/merge conflicts и ожидает только human approval/final merge. Выкладка и live readback выполняются после human merge; невозможный из-за общих release blockers APK acceptance остаётся явно записанным, а не считается пройденным.
