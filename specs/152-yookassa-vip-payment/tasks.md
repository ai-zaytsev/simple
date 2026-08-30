# Tasks: Оплата VIP через ЮKassa

- [x] Создать feature branch и feature-memory от актуального `main`
- [x] Проверить официальные API, webhook, test-store и fee contracts ЮKassa
- [x] Проверить наличие `YOOKASSA_TEST_SHOP_ID` и `YOOKASSA_TEST_SECRET_KEY` без чтения значений
- [x] Рассчитать и зафиксировать цены 1/3/12 месяцев
- [x] Зафиксировать отсутствие legacy workspace dependency
- [x] Добавить каталог продуктов, payment schema и срочный VIP
- [x] Реализовать общий provider contract и adapter ЮKassa
- [x] Реализовать Core create/status/webhook API и однократную активацию
- [x] Подключить Core secrets в deploy workflow
- [x] Реализовать Android external checkout без SDK
- [x] Добавить нейтральную return page в публичный Core route
- [x] Добавить тесты всех acceptance scenarios
- [x] Обновить architecture/integration/privacy/secrets docs и tech debt
- [x] Выполнить локальную валидацию, включая чистый PostgreSQL lifecycle test
- [ ] Создать PR и пройти PR loop до merge-ready
- [ ] После human merge развернуть и выполнить live test-store matrix
