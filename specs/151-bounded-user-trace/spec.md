# Spec: Ограниченный пользовательский destination trace

- Feature ID: `151-bounded-user-trace`
- Feature Branch: `feature/151-bounded-user-trace`
- Status: `active`

## Goal

Довести уже существующую запись подробного Android-лога до требования Business Owner:

- без явного нажатия пользователя приложение не записывает адреса сайтов и сервисов, к которым обращалось устройство;
- повторная команда запуска не может отменить первоначальный десятиминутный предел;
- после окончания записи пользователь может повторно открыть приложение и отправить, сохранить или удалить готовый файл.

Обычные операционные события, Xray на уровне `warning`, bridge на уровне `warning` и Android logcat допустимы, пока они не содержат destination пользователя.

## Business Owner Decision

Решение уточнено 2026-08-30:

- локальная запись destination разрешена только после информированного нажатия кнопки;
- после передачи файла почтовому приложению локальная копия удаляется; если письмо не дойдёт, пользователь записывает лог заново;
- исправления таймера и доступности завершённого файла выполняются отдельной задачей.

## Non-Goals

- не удалять обычные технические логи, которые не содержат destination пользователя;
- не менять почтовый канал, момент локальной очистки после передачи письма или системный сценарий `CreateDocument`;
- не менять VPN-маршрутизацию, Control Plane, аналитику или APK publication flow;
- не включать в задачу оставшиеся VIP и entry-path release blockers.

## Scope

- `android/app/src/main/kotlin/download/simplevpn/vpn/` — идемпотентное окно записи и восстановление состояния готового файла;
- `android/app/src/main/kotlin/download/simplevpn/MainActivity.kt` — восстановление доступности готовой записи без удаления на старте;
- `android/app/src/test/` — регрессии повторного запуска, deadline и восстановления;
- `docs/architecture/privacy-model.md`, `docs/architecture/android-client.md`, `docs/release-blockers.md` — актуальный privacy contract и удаление устаревшего блокера;
- `specs/151-bounded-user-trace/` — task memory.

## Repository Memory Updates

- privacy model должен различать запрещённую автоматическую историю и ограниченную локальную запись после информированного действия;
- Android architecture должна назвать единственную runtime-точку включения `info` и правила жизненного цикла trace;
- устаревший блокер «Убрать отладочную обвязку слайса» должен исчезнуть только после реализации и проверок этой задачи.

## Legacy Workspace Assessment

Альтернативная workspace-механика не требуется. Проверено перед стартом: локальный `main` и `origin/main` совпадали на `4d35aa42417ce373ef7c4497194bc0d3e4b097e5`, working tree был чистым. Работа выполняется через одну feature branch и один PR.

## Acceptance Criteria

- `EngineLog.Trace` остаётся единственным режимом Xray, который может включить `info`; access log выключен во всех режимах;
- единственный product-path запуска destination trace начинается с явной кнопки и предупреждения в UI;
- первый start создаёт deadline не позднее чем через 10 минут;
- повторный start во время записи не меняет deadline и не снимает запланированную остановку;
- выключение VPN по-прежнему завершает запись;
- после автоматической или ручной остановки готовая запись переживает повторное создание Activity и перезапуск процесса;
- готовая запись исчезает только после текущих явных действий отправить, успешно сохранить или удалить;
- почтовый сценарий и его политика очистки не меняются;
- устаревший logging blocker удалён, а новое разрешение Business Owner записано в durable docs;
- текущий PR head проходит required checks и не имеет merge conflicts; final merge остаётся за человеком.

## Validation

- unit tests чистой модели окна записи: первый start, повторный start, stop и новый start;
- unit tests восстановления `TraceState.Ready` из существующего файла без потери активного `Recording`;
- существующие `EngineLoggingTest`, `TraceWarningTest`, `SharedFileNameTest`, `OneLetterTest`;
- Android `testDebugUnitTest` и `assembleDebug` в GitHub Actions;
- `git diff --check`, process baseline, PR guard и AI review/fallback.
