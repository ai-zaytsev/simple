# Tasks: Проверка ЮKassa и возвраты VIP

- [x] Создать feature branch от актуального `main`
- [x] Создать active feature-memory и зафиксировать отсутствие legacy workspace dependency
- [x] Проверить существующие payment/entitlement/provider/Android contracts
- [x] Проверить официальные refunds API, webhook и 24-часовой idempotency contract
- [ ] Проверить в test store ограничения реально используемых способов оплаты
- [x] Зафиксировать точную policy math и refund state machine
- [x] Добавить schema/store модель возвратов и provider attempts
- [x] Расширить provider contract и YooKassa adapter
- [x] Реализовать Core refund orchestration, canonical reconciliation и recovery потерянного ответа
- [x] Добавить Core API и Android flow возврата
- [ ] Покрыть payment/refund/boundary/error сценарии тестами
- [x] Обновить architecture/integration/API docs, BO-инструкцию и tech debt
- [ ] Выполнить локальную валидацию, включая чистый PostgreSQL lifecycle
- [ ] Создать PR и пройти PR loop до merge-ready
- [ ] После human merge развернуть и выполнить live test-store matrix
- [ ] Сверить живые payment/refund/VIP состояния после deploy
