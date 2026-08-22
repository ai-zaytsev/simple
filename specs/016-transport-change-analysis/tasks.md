# Tasks: Transport Change Analysis

- [x] Удалить проверочную ноду, подтвердить отсутствие дроплетов
- [x] Найти все места фиксации REALITY
- [x] Написать `docs/architecture/transport-change.md`
- [x] Заменить `ADR-014` на `ADR-022`
- [x] Обновить карту документов

## Требует Business Owner

- [ ] Что за сайт открывается на entry-доменах
- [ ] Подтвердить схему с wildcard и централизованным выпуском
- [ ] Открывать ли порт 80 под редирект
- [ ] Убирать REALITY полностью или оставить как второй транспорт

## После Решений

- [ ] Обновить threat-model, bootstrap-recovery, node-lifecycle, mvp-topology, secrets-model, stack
- [ ] Перевести `ConnectionProfile` и `XrayConfigBuilder` на `ws` + `tls`
- [ ] Переписать cloud-init: Nginx, сайт, доставка сертификата
- [ ] Прогнать проверочную ноду по новой схеме
