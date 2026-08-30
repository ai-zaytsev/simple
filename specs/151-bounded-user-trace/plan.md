# Plan: Ограниченный пользовательский destination trace

- Feature ID: `151-bounded-user-trace`
- Feature Branch: `feature/151-bounded-user-trace`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать уточнённое требование и границы задачи в feature-memory.
2. Вынести переходы окна записи в чистую Kotlin-модель с неизменяемым deadline при повторном start.
3. Подключить модель к `SimpleVpnService`, сохранив остановку через 10 минут и при выключении VPN.
4. Восстанавливать `TraceState.Ready` по существующему trace-файлу вместо удаления файла в `MainActivity.onCreate`.
5. Добавить регрессионные unit tests для deadline и восстановления состояния.
6. Обновить privacy/Android docs и удалить устаревший release blocker.
7. Выполнить локальные доступные проверки, затем PR loop и AI review/fallback.

## Design

Состояние активного окна моделируется отдельно от Android `Handler`. Первый start возвращает новый deadline; повторный start возвращает `AlreadyRunning` с тем же deadline и не даёт сервису трогать callback. Stop очищает активное окно. Такой контракт проверяется без Android runtime.

При создании Activity `VpnController` восстанавливает `Ready`, если подробный файл существует и текущий state не `Recording`. Файл не удаляется автоматически. Удаление остаётся только в существующих явных путях отправки, успешного сохранения и кнопки удаления.

## Risks

- рассинхронизация чистой модели и `Handler` может вернуть бесконечную запись; сервис должен менять callback только после реального перехода состояния;
- восстановление не должно превращать активную запись в `Ready` при обычном пересоздании Activity;
- бессрочно забытый готовый файл повышает локальный privacy-риск, но это следует из требования «после окончания можно отправить или сохранить» и явного решения не удалять его на старте;
- broad release-blocker нельзя удалить до того, как тесты удерживают новый более точный контракт.

## Validation Plan

- проверить все callers `ACTION_TRACE_START`, `EngineLog.Trace`, `traceFile`, `dropTrace`;
- запустить доступные локальные source checks и Kotlin tests, если toolchain присутствует;
- GitHub Android Build обязан выполнить `testDebugUnitTest` и `assembleDebug`;
- required checks должны быть зелёными на текущем PR head;
- AI Review должен отработать либо явно перейти в documented human fallback.

## Merge Readiness

Задача готова только после PR loop: green required checks, отсутствие blocking findings и merge conflicts; human остаётся final merge authority.
