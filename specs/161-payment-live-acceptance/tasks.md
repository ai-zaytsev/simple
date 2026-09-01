# Tasks: Живая приемка платежей и возвратов

- [x] Создать feature branch от merged main
- [x] Создать и заполнить active feature-memory
- [x] Зафиксировать отсутствие legacy workspace dependency
- [x] Подтвердить live full refund: 399 ₽, VIP→FREE, external off, node `2→1`
- [x] Вернуть purchases FREE period с тестового 1 дня на продуктовые 7
- [x] Проверить актуальные официальные webhook/refund/test-store contracts
- [x] Добавить redacted DB/provider payment/refund readback
- [x] Добавить строго test-only подготовку partial-refund времени
- [x] Добавить process/source/fixture tests
- [x] Обновить durable docs и связанные feature-memory
- [x] Выполнить локальную валидацию
- [x] Создать PR и пройти PR loop (PR #167 merged)
- [ ] После human merge выполнить оставшуюся live matrix
- [x] Настроить webhook в кабинете test store (`payment.succeeded`, `payment.canceled`, `refund.succeeded`)
- [x] Повторить payment/refund webhook по два раза: HTTP 200, applied=false, durable state unchanged
- [ ] Вернуть и подтвердить purchases FREE period 7 дней после тестов
