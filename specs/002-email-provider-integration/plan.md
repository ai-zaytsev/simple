# Plan: Email Provider Integration

- Feature ID: `002-email-provider-integration`
- Feature Branch: `feature/002-email-provider-integration`
- Owner: `orchestrator`

## Implementation Slices

1. Переписать `email-provider-check.yml` в ASCII без входных параметров.
2. Смержить и убедиться, что GitHub зарегистрировал workflow.
3. Запустить проверку, получить `messageId` и события доставки.
4. Зафиксировать причину отказа первой версии в `docs/integrations/brevo.md`.

## Risks

- причина отклонения первой версии установлена методом исключения, а не по сообщению GitHub: если после упрощения workflow зарегистрируется, точный виновник останется неизвестным
- события доставки провайдер отдаёт с задержкой, поэтому пустой результат опроса не означает недоставку
- попадание письма во «Входящие» или в спам автоматически не определяется и остаётся за человеком

## Validation Plan

- YAML-валидация и проверка на отсутствие не-ASCII
- регистрация workflow после merge
- фактический запуск с отправкой на два ящика
- PR loop зелёный

## Merge Readiness

Задача закрывается, когда workflow зарегистрирован, запущен, письма приняты провайдером, а технический статус передан Business Owner.
