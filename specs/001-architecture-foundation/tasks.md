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
- [ ] Получить от Business Owner решения со статусом `needs-owner-decision`
- [ ] Merge PR — остаётся за человеком

## Что Осталось Открытым

Пять решений в `docs/architecture/decisions.md` имеют статус `needs-owner-decision`: канал `rescue` (ADR-004), VPS-провайдер (ADR-005), платёжный провайдер (ADR-006), email-провайдер (ADR-012), DNS-резолвер (ADR-008).

Ни одно из них не блокирует переход к следующей стадии: архитектура спроектирована так, чтобы работать при любом исходе. Запросы собраны в `docs/architecture/prerequisites.md`.
