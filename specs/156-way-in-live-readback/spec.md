# Spec: Live readback проверки путей входа

- Feature ID: `156-way-in-live-readback`
- Feature Branch: `feature/156-way-in-live-readback`
- Status: `merge-ready`

## Goal

Довести уже смерженное исправление device probes до живой приёмки: сохранить факт восстановления edge, развернуть Core, передать test APK на целевое Android-устройство и увидеть в панели проверки каждого включённого пути из пользовательской сети.

## Acceptance Criteria

- Документация отражает восстановление `n-c5e6` и четыре отвечающих bootstrap route.
- Core с allowlist всех включённых entry развёрнут из merged `main`.
- CI test APK с device sweep доступен для установки.
- После запуска APK на устройстве из России панель показывает device checks для всех четырёх entry.
- Только после live readback релизный блокер помечен закрытым.

## Non-Goals

- Публикация официального APK.
- Подъём новой ноды или покупка домена.
