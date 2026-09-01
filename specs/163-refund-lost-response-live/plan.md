# Plan: Живая проверка потерянного ответа возврата

- Feature ID: `163-refund-lost-response-live`
- Feature Branch: `feature/163-refund-lost-response-live`
- Owner: `orchestrator`

## Implementation

1. Расширить private snapshot полями attempt idempotency/created time.
2. Добавить test-only provider retry verifier и manual workflow.
3. Проверить тот же ID, metadata, сумму, list cardinality и durable before/after.
4. Добавить redaction/guard tests и durable docs.
5. Пройти PR loop; после human merge выполнить live retry до 24-часовой границы.

## Risks

- второй денежный refund: предотвращается тем же key и exact payload, затем
  проверяется provider list cardinality и DB aggregate;
- утечка private IDs/key: только runner files/environment, safe report prefixes;
- поздний повтор: verifier fail closed до network POST;
- ошибочный payment: UUID prefix и test-store/succeeded predicates.

## Merge Readiness

Только current PR head с green checks, без findings/conflicts. После merge
обязательны live provider readback и Read The Panel.
