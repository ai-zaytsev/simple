# Plan: DNS State Record

- Feature ID: `005-dns-state-record`
- Feature Branch: `feature/005-dns-state-record`
- Owner: `orchestrator`

## Implementation Slices

1. Обновить раздел текущего состояния в `docs/integrations/dns.md`.
2. Добавить правило проверки Cloudflare-токена.
3. Закрыть задачи стадий 003 и 004.

## Risks

- состояние DNS меняется, а документ устаревает: раздел помечен датой установления факта
- место хранения зоны `SITE_DOMAIN` остаётся неизвестным и станет проблемой только в момент срочной правки

## Validation Plan

- PR loop зелёный
- перекрёстные ссылки не ведут в несуществующие файлы

## Merge Readiness

Документальная задача, закрывается вместе с merge.
