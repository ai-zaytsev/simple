# Spec: Надёжность публикации обновлений

- Feature ID: `154-app-update-hardening`
- Feature Branch: `feature/154-app-update-hardening`
- Status: `merge-ready`

## Goal

Закрыть два blocking finding, найденных после human merge стадии обновлений: не допускать привязки артефакта предыдущей версии к новому общему `latest` при нескольких каналах и позволить штатно повторить синхронизацию Core, если публичный immutable APK уже опубликован.

## Acceptance Criteria

- Продвижение нового `versionCode` начинает новый набор channel material; другие каналы прикрепляются только к той же версии.
- Добровольный update UI не показывается, пока текущий канал не имеет материала для `latest`; обязательный minimum при этом не ослабляется.
- Не-direct канал может продвинуть общую версию без выдуманного APK URL/hash.
- Повтор `Publish APK` для точной уже опубликованной версии не перезаписывает объект, проверяет публичные SHA-256 и release certificate и повторяет Core synchronization.
- PostgreSQL lifecycle, Go tests/vet, actionlint, Android Build и PR checks зелёные на текущем head.

## Non-Goals

- Реализация Google Play executor.
- Публикация нового APK при открытых общих release blockers.
- Изменение latest/min verdict или существующего API Android.
