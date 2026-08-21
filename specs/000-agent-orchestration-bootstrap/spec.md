# Spec: Agent Orchestration Bootstrap

- Feature ID: `000-agent-orchestration-bootstrap`
- Feature Branch: `feature/000-agent-orchestration-bootstrap`
- Status: `implemented`

## Goal

Развернуть в репозитории VPN-приложения тот же переносимый branch-based process-layer, что используется в `telegram_proxy`: agent orchestration, PR loop, AI review и repository memory — без продуктовой логики и без проектирования самого приложения.

## Non-Goals

- не проектировать архитектуру VPN-приложения
- не переносить бизнес-логику из `telegram_proxy`
- не создавать продуктовые каталоги (`src/`, `tests/`, `ansible/` и т.д.) до появления реальной задачи
- не строить flow вокруг отдельной workspace-механики

## Scope

- корневые process contracts: `AGENTS.md`, `CLAUDE.md`
- `.specify/` templates
- `docs/` process documentation
- `specs/000-agent-orchestration-bootstrap/` как feature-memory
- `scripts/*.ps1`
- `.github/workflows/*.yml`
- `.claude/settings.local.json`

## Repository Memory Updates

- durable memory: `docs/README.md`, `docs/ai-pr-workflow.md`, `docs/worker-orchestration.md`, `docs/environment.md`
- task-memory: `specs/000-agent-orchestration-bootstrap/`

## Legacy Workspace Assessment

Репозиторий создан с нуля, versioned-истории до bootstrap не существует. Зависимости от альтернативной workspace-механики нет по построению, поэтому branch-based flow вводится как единственный путь `task -> branch -> PR -> checks -> AI review -> merge-ready`.

## Отличия От Донорского Репозитория

- `scripts/Send-TelegramNotification.ps1` не переносится: он завязан на runtime `telegram_proxy` (`src/notifications.py`, контейнер `proxy_app`) и не является частью переносимого process-layer
- `Baseline Checks` в шаге компиляции Python работает адаптивно: при отсутствии `src/` и `tests/` шаг явно пропускается, а не падает
- `docs/` не содержит продуктовых разделов донора

## Acceptance Criteria

- новая задача заводится через `scripts/New-FeatureBranch.ps1` и создаёт `specs/<feature-id>/` из шаблонов
- implementation worker выбирается и запускается без отдельной workspace-механики
- PR loop задокументирован как completion contract
- AI review запускается через GitHub workflow с repo variable `AI_REVIEW_AGENT` и имеет documented fallback
- process-layer отделён от будущего продуктового кода

## Validation

- PowerShell scripts проходят syntax parse под Windows PowerShell 5.1
- локальный прогон полного скриптового flow на тестовом feature-id
- workflow layer содержит стабильные required check names: `Process Baseline`, `PR Loop Guard`, `AI Review`
- process docs/specs присутствуют и согласованы между собой
