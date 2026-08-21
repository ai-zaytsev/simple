# Tasks: Agent Orchestration Bootstrap

- [x] Инициализировать git-репозиторий с веткой `main`
- [x] Создать и заполнить `spec.md`
- [x] Создать и заполнить `plan.md`
- [x] Зафиксировать проверку legacy workspace dependency
- [x] Перенести process contracts (`AGENTS.md`, `CLAUDE.md`)
- [x] Перенести `.specify/` templates
- [x] Добавить process docs (`docs/README.md`, `ai-pr-workflow.md`, `worker-orchestration.md`, `environment.md`)
- [x] Добавить локальные orchestration scripts
- [x] Добавить GitHub workflow layer и адаптировать `Baseline Checks`
- [x] Добавить `.claude/settings.local.json`
- [x] Выполнить локальную валидацию скриптового flow
- [x] Подключить `origin` (`ai-zaytsev/simple`, private) и запушить `main`
- [x] Установить repo variable `AI_REVIEW_AGENT` = `claude`
- [x] Проверить `Process Baseline` и `AI Review` через `workflow_dispatch`
- [x] Проверить `PR Loop Guard` и `Publish-FeaturePR.ps1` на реальном PR
- [ ] Пометить `Process Baseline`, `PR Loop Guard`, `AI Review` как required checks — заблокировано планом GitHub, см. `docs/environment.md`
