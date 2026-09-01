# Plan: Диагностика доступа к APK-сайту

- Feature ID: `174-site-access-preflight`
- Feature Branch: `feature/174-site-access-preflight`
- Owner: `orchestrator`

## Implementation Slices

1. Добавить read-only fingerprint preflight существующего site key и CI key.
2. Выполнить preflight с feature branch и сопоставить его только с локальными
   public-key fingerprint.
3. Если исходный ключ найден, открыть SSH на ограниченное время только для
   текущего внешнего `/32`, добавить существующий CI public key и закрыть окно.
4. Повторить штатный content deploy, проверить firewall, Cloudflare и live page.
5. Пройти PR loop и записать live result.

## Risks

- зарегистрированный DO key может отсутствовать среди локальных private keys;
- maintenance window должен закрываться trap даже при ошибке;
- нельзя считать сам возврат HTTP 200 подтверждением нового content.

## Validation Plan

- Python contract test deploy workflow;
- feature-ref dry run с чтением fingerprint;
- live deploy, firewall API verification и public DOM/readback.

## Merge Readiness

Задача считается готовой только после PR loop: green required checks, no blocking findings, no merge conflicts, human approval pending or merge pending.

