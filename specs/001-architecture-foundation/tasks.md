# Tasks: MVP Architecture Foundation

- [x] Создать и заполнить `spec.md`
- [x] Создать и заполнить `plan.md`
- [x] `docs/architecture/overview.md` — диаграмма и границы компонентов
- [x] `docs/architecture/threat-model.md`
- [x] `docs/architecture/privacy-model.md`
- [x] `docs/architecture/identity-model.md`
- [x] `docs/architecture/secrets-model.md`
- [x] `docs/architecture/remote-config.md`
- [x] `docs/architecture/bootstrap-recovery.md`
- [x] `docs/architecture/node-lifecycle.md`
- [x] `docs/architecture/entitlement-model.md`
- [x] `docs/architecture/observability.md`
- [x] `docs/architecture/infrastructure.md`
- [x] `docs/architecture/failure-scenarios.md`
- [x] `docs/architecture/decisions.md`
- [x] `docs/architecture/prerequisites.md`
- [x] `docs/architecture/README.md` — карта документов и прослеживаемость принципов ТЗ
- [x] Обновить `docs/README.md`
- [x] Проверить перекрёстные ссылки между документами
- [x] Прогнать PR loop

## Второй Заход: Решения Business Owner

- [x] Агрегированный retention через механизм когорт, `ADR-003` закрыт
- [x] Канал `rescue` спроектирован и переведён в обязательный до публичного запуска, `ADR-004` закрыт
- [x] `docs/architecture/evolution.md` — инварианты И-1 – И-16 для резервирования Core и второго транспорта
- [x] `ADR-011` и `ADR-014` переведены в `accepted (temporary)` с привязкой к инвариантам
- [x] Оплата убрана из MVP, `ADR-006` переписан, платёжные разделы удалены из документов и prerequisites
- [x] Разделение нод FREE/VIP убрано; пулы переопределены как `app`, `export`, `edge`, `quarantine`
- [x] `ADR-002` пересмотрен: изоляция FREE/VIP больше не является контролем
- [x] Экспорт конфигурации для VIP через subscription-ссылку, `ADR-017`
- [x] Схема плана переведена на конверт транспорта, добавлено согласование возможностей клиента
- [x] Добавлены поля `cohort` и `transport_kind` в модель аналитики
- [x] Email-провайдер: Brevo зафиксирован, `ADR-012` закрыт, добавлены `docs/integrations/brevo.md` и workflow `Email Provider Check`
- [ ] Business Owner: перевыпустить ключ Brevo и задать секреты
- [ ] Business Owner: подтвердить домен отправителя, настроить SPF, DKIM, DMARC
- [ ] Проверить доставляемость на реальных ящиках `mail.ru` и `yandex`
- [ ] Merge PR — остаётся за человеком

## Что Осталось Открытым

Два решения со статусом `needs-owner-decision`: VPS-провайдер (`ADR-005`) и DNS-резолвер (`ADR-008`). Email-провайдер выбран: Brevo. Резервный email-провайдер не выбран и обязателен до публичного запуска.

Проектирование они не блокируют. Развёртывание блокирует выбор VPS-провайдера. Запросы собраны в `docs/architecture/prerequisites.md`.
