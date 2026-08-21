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

Локально установлен только Store-alias `python.exe`, реального интерпретатора нет. Для process-layer это не нужно и в стек MVP Python не входит, поэтому устанавливать его не требуется.

## Требует Ручных Действий

Текущее состояние GitHub-настроек — в разделе `Состояние GitHub` ниже. Контракт по подключению remote описан в `docs/worker-orchestration.md`, раздел `Remote And Repository Settings`.

## Инварианты Среды

- process-layer не должен зависеть от `pwsh` или любого локального CLI сверх `git`
- продуктовый тулчейн (Go, Terraform, Android) не является требованием process-layer
- `gh` опционален: `Publish-FeaturePR.ps1` при его отсутствии пушит ветку и печатает compare-URL для ручного создания PR
- отсутствие сконфигурированного AI reviewer переводит review authority к человеку, а не имитирует review

## Состояние GitHub

| Настройка | Значение |
| --- | --- |
| Repository | `ai-zaytsev/simple`, public |
| Default branch | `main`, protected |
| Required checks | `Process Baseline`, `PR Loop Guard`, `AI Review`, strict |
| Pull request | обязателен для `main`, required approvals — `0` |
| Force push / branch deletion | запрещены |
| `enforce_admins` | `false` |
| Repo variable `AI_REVIEW_AGENT` | `claude` |

Почему `required_approving_review_count` = `0`: в репозитории один владелец, а GitHub не позволяет апрувить собственный PR. При значении `1` merge собственного PR был бы заблокирован. Нулевое значение сохраняет обязательный PR-flow и обязательные checks, при этом merge остаётся ручным действием человека — то есть completion contract из `AGENTS.md` не нарушен.

Branch protection стала доступна после перевода репозитория в public: на free-плане для приватных репозиториев и classic branch protection, и rulesets отвечают `403 Upgrade to GitHub Pro`.

Если репозиторий потребуется вернуть в private, защита `main` отключится вместе с этим — тогда инварианты процесса снова будут держаться только на `AGENTS.md`.

## Тулчейн Под Стек MVP

Стек зафиксирован в `docs/stack.md`. Локально пока не установлено ничего из продуктового тулчейна — это не мешает process-layer, потому что `Baseline Checks` выполняется на GitHub runner и пропускает шаги, пока соответствующего кода нет.

Устанавливать по мере появления задач:

```
winget install --id GoLang.Go --source winget
winget install --id Hashicorp.Terraform --source winget
```

Android-разработка потребует Android Studio и JDK; ставить их до первой Android-задачи не нужно.
