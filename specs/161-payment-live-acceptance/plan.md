# Plan: Живая приемка платежей и возвратов

- Feature ID: `161-payment-live-acceptance`
- Feature Branch: `feature/161-payment-live-acceptance`
- Owner: `orchestrator`

## Implementation Slices

1. Зафиксировать доказанный full-refund результат и остаток live matrix.
2. Добавить redacted payment/refund readback из PostgreSQL и canonical YooKassa GET.
3. Добавить строго ограниченный `prepare_partial` для test payment с возрастом policy 8 суток.
4. Покрыть выбор цели, запреты mutation и redaction тестами.
5. Пройти PR loop и human merge.
6. Выполнить payment failure/cancel, webhook/repeat и partial-refund live matrix.
7. Вернуть purchases FREE period к 7 дням и обновить единый BO status.

## Risks

- operator test tool может затронуть реальную оплату — mutation требует `provider_test=true`, VIP, succeeded/refundable, отсутствия refund и ровно одного account match;
- provider identifiers могут утечь в публичный Actions log — запросы живут в temp JSON, renderer печатает только короткие внутренние prefixes и проверяемые поля;
- повтор подготовки времени может накапливать сдвиг — timestamps выставляются относительно `now()`, а не уменьшаются повторно;
- DB status может расходиться с provider — readback явно показывает обе стороны и verdict, а не выбирает удобную;
- webhook требует личного кабинета HTTP Basic Auth — workflow не пытается заменить это API-вызовом OAuth.

## Validation Plan

- parse workflow and script tests;
- source assertions на test-only guards и отсутствие опасных полей в output;
- local baseline subset и `git diff --check`;
- PR checks current head;
- live readback before/after controlled partial refund;
- post-change panel/node credentials and purchases settings readback.

## Merge Readiness

Merge-ready требует green checks текущего PR head, отсутствия blocking findings и конфликтов. Human выполняет merge. Стадия ЮKassa закрывается только после оставшейся живой матрицы и webhook dashboard action.
