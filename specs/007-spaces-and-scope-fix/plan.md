# Plan: Spaces Configuration And Scope Verdict Fix

- Feature ID: `007-spaces-and-scope-fix`
- Feature Branch: `feature/007-spaces-and-scope-fix`
- Owner: `orchestrator`

## Implementation Slices

1. Исправить вердикт `Infra Inventory`.
2. Зафиксировать правило проверки доступа в `docs/integrations/digitalocean.md`.
3. Добавить `Spaces Configure` с режимом dry run.
4. Смержить, прогнать инвентаризацию, затем dry run, затем apply.

## Risks

- совместимость S3 у DigitalOcean неполная: форма lifecycle с `Filter` может быть отклонена, поэтому предусмотрен откат на устаревшую форму с `Prefix`
- versioning на bucket с бэкапами копит старые версии, поэтому срок их жизни ограничен отдельным правилом
- round-trip создаёт временный объект: он удаляется в том же шаге, а префикс `healthcheck/` отделён от рабочих

## Validation Plan

- локальная проверка YAML, ASCII и корректности встроенного JSON
- dry run перед apply
- проверка versioning и списка правил после apply

## Merge Readiness

Задача закрывается, когда bucket настроен и проверен round-trip, а инвентаризация проходит на текущем токене.
