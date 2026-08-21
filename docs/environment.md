# Environment

Состояние локальной среды, в которой работает process-layer этого репозитория. Проверено 2026-08-21.

## Проверено И Работает

| Компонент | Статус | Значение |
| --- | --- | --- |
| `git` | OK | 2.54.0.windows.1 |
| `gh` (GitHub CLI) | OK | 2.93.0, `C:\Program Files\GitHub CLI\gh.exe` |
| `gh auth` | OK | аккаунт `ai-zaytsev`, scopes: `gist`, `read:org`, `repo`, `workflow` |
| Windows PowerShell | OK | 5.1.26100 — все `scripts/*.ps1` парсятся и выполняются |
| `node` | OK | присутствует, process-layer его не требует |

Прогон полного локального flow (`New-FeatureBranch` -> `Select-ImplementationAgent` -> `Start-ImplementationWorker` -> `Invoke-AIReview -Mode prepare`) выполнен на тестовом feature-id и завершился успешно. Тестовые артефакты удалены.

## Отсутствует, Но Не Блокирует

### PowerShell 7 (`pwsh`)

Не установлен. Скрипты рассчитаны на fallback и корректно работают под Windows PowerShell 5.1 (`Get-PreferredPowerShellCommand` возвращает `powershell`).

Установка при желании:

```
winget install --id Microsoft.PowerShell --source winget
```

### Python

Локально установлен только Store-alias `python.exe`, реального интерпретатора нет. Для process-layer это не нужно: `Baseline Checks` выполняется на GitHub runner, а шаг компиляции пропускается, пока в репозитории нет `src/` или `tests/` с `.py`.

Python понадобится только если стек VPN-приложения окажется Python. Тогда:

```
winget install --id Python.Python.3.12 --source winget
```

## Требует Ручных Действий

Required checks и branch protection — см. раздел `Ограничение Плана GitHub` ниже. Остальные шаги подключения remote описаны в `docs/worker-orchestration.md`, раздел `Remote And Repository Settings`.

## Инварианты Среды

- process-layer не должен зависеть от `pwsh`, `python` или любого локального CLI сверх `git`
- `gh` опционален: `Publish-FeaturePR.ps1` при его отсутствии пушит ветку и печатает compare-URL для ручного создания PR
- отсутствие сконфигурированного AI reviewer переводит review authority к человеку, а не имитирует review

## Ограничение Плана GitHub

Репозиторий `ai-zaytsev/simple` приватный на free-плане. И classic branch protection, и rulesets возвращают:

```
403 Upgrade to GitHub Pro or make this repository public to enable this feature.
```

Следствие: required checks и запрет direct push в `main` сейчас держатся только на уровне процесса (`AGENTS.md`), а не принудительно на стороне GitHub. Checks запускаются на каждом PR и видны, но технически не блокируют merge.

Как снять ограничение — любой из вариантов:

- сделать репозиторий публичным: `gh repo edit ai-zaytsev/simple --visibility public --accept-visibility-change-consequences`
- перейти на GitHub Pro

После этого включить защиту:

```
gh api --method PUT repos/ai-zaytsev/simple/branches/main/protection --input protection.json
```

где `protection.json` содержит `required_status_checks.contexts` со значениями `Process Baseline`, `PR Loop Guard`, `AI Review`.
