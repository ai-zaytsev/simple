# Plan: Live readback проверки путей входа

- Feature ID: `156-way-in-live-readback`
- Feature Branch: `feature/156-way-in-live-readback`
- Owner: `orchestrator`

1. Перенести post-merge live findings в отдельный follow-up PR.
2. Развернуть merged Core и проверить публичные route.
3. Передать подписанный CI test APK на целевое устройство.
4. После запуска прочитать панель и закрыть блокер только по живым device checks.

