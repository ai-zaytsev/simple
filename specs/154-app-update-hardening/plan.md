# Plan: Надёжность публикации обновлений

- Feature ID: `154-app-update-hardening`
- Feature Branch: `feature/154-app-update-hardening`
- Owner: `orchestrator`

1. Перенести review-fixes на отдельную ветку от merged `main`.
2. Сделать channel material строго относящимся к текущему latest и покрыть multi-channel lifecycle.
3. Добавить idempotent retry публично завершённой публикации до Core synchronization.
4. Проверять hash и установленный release certificate через официальный домен.
5. Пройти локальную, PostgreSQL, Android и PR-валидацию.
6. После human merge развернуть Core и выполнить live readback вместе с основной стадией.

