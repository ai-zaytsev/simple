# Plan: Agent Orchestration Bootstrap

- Feature ID: `000-agent-orchestration-bootstrap`
- Feature Branch: `feature/000-agent-orchestration-bootstrap`
- Owner: `orchestrator`

## Implementation Slices

1. Инициализировать git-репозиторий с базовой веткой `main`.
2. Перенести переносимую часть process-layer из `telegram_proxy`: contracts, `.specify/`, `scripts/`, `docs/`.
3. Адаптировать GitHub workflow layer под репозиторий без продуктового кода.
4. Зафиксировать feature-memory bootstrap-задачи и состояние локальной среды.
5. Прогнать локальный flow целиком и удалить тестовые артефакты.
6. Подключить remote и включить required checks.

## Risks

- смешивание process-layer с будущим продуктовым кодом
- перенос продуктовых допущений донора (Python-стек, Telegram-нотификации) в проект, где стек ещё не выбран
- неявная зависимость от локальных CLI (`gh`, `pwsh`, `python`), которых нет в среде
- CI, падающий на пустом репозитории из-за проверок продуктового кода

## Validation Plan

- парсинг всех `scripts/*.ps1` через `System.Management.Automation.Language.Parser`
- локальный прогон `New-FeatureBranch -> Select-ImplementationAgent -> Start-ImplementationWorker -> Invoke-AIReview`
- проверка наличия всех required process artifacts из `Baseline Checks`
- проверка доступности `git` и `gh` в локальной среде

## Merge Readiness

Задача считается готовой только после PR loop: green required checks, no blocking findings, no merge conflicts, human approval pending or merge pending.

До подключения remote bootstrap остаётся в состоянии `local-ready`, а не `merge-ready`.
