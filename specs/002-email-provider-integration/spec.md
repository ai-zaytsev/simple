# Spec: Email Provider Integration

- Feature ID: `002-email-provider-integration`
- Feature Branch: `feature/002-email-provider-integration`
- Status: `in-progress`

## Goal

Довести интеграцию с email-провайдером Brevo до состояния, в котором доставляемость проверяется автоматизированно и без раскрытия секретов, а требования к backend-части зафиксированы для стадии реализации Control Plane.

## Non-Goals

- не реализовывать Control Plane: его ещё не существует
- не настраивать webhook в панели провайдера: он требует публичного HTTPS-эндпоинта Control Plane
- не выбирать резервного провайдера за Business Owner

## Scope

- `.github/workflows/email-provider-check.yml`
- `docs/integrations/brevo.md`
- `specs/002-email-provider-integration/`

## Проблема, Ради Которой Заведена Задача

Первая версия `email-provider-check.yml` прошла merge в `main`, но GitHub Actions её не зарегистрировал: `gh workflow run` отвечал `404 not found on the default branch`, страница workflow показывала `This workflow does not exist`, при этом файл присутствовал на `main` и проходил YAML-валидацию локально.

Ошибку парсинга GitHub не публиковал ни в Actions UI, ни через API, ни как `startup_failure`. Отладка возможна только методом исключения.

Отличия отклонённого файла от трёх работающих workflow репозитория:

- не-ASCII символы в `name` шагов и в `description` входного параметра
- `workflow_dispatch` с input типа `boolean`
- обращение к контексту `inputs` в step-level `if`

## Решение

Файл переписан так, чтобы не отличаться от работающих workflow ничем, кроме содержания:

- весь файл в ASCII, включая имена шагов и текст сообщений
- входные параметры убраны, `workflow_dispatch` без `inputs`
- обращения к контексту `inputs` убраны
- функциональность сохранена полностью

## Acceptance Criteria

- workflow зарегистрирован GitHub и запускается через `workflow_dispatch`
- запуск отправляет по письму на `EMAIL_TEST_GMAIL` и `EMAIL_TEST_YANDEX`
- в summary попадают `messageId` и события доставки по каждому получателю
- в публичных логах отсутствуют ключ, адреса получателей и данные аккаунта
- причина отказа первой версии зафиксирована в документации, чтобы не повторить её в следующих workflow

## Validation

- локальная YAML-валидация и проверка на отсутствие не-ASCII
- фактическая регистрация workflow в GitHub Actions
- фактический запуск с отправкой писем
- `Process Baseline`, `PR Loop Guard`, `AI Review` зелёные
