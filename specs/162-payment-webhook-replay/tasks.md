# Tasks: Безопасная повторная доставка webhook ЮKassa

- [x] Создать и заполнить `spec.md`, `plan.md`, `tasks.md`
- [x] Зафиксировать отсутствие legacy workspace dependency
- [x] Добавить redacted webhook replayer/validator
- [x] Добавить manual GitHub workflow
- [x] Добавить unit tests и Process Baseline step
- [x] Обновить durable BO/integration docs
- [x] Выполнить локальную валидацию: 6 replay tests, 4 acceptance tests, `git diff --check`
- [x] Опубликовать PR #168 и получить green checks/AI review
- [x] После human merge выполнить live replay: 4 × HTTP 200, все applied=false
- [x] Прочитать панель сразу после replay: FREE=1, VIP=0; итоговое восстановление 7 дней зафиксировано в задаче 161
