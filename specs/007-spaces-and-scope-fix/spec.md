# Spec: Spaces Configuration And Scope Verdict Fix

- Feature ID: `007-spaces-and-scope-fix`
- Feature Branch: `feature/007-spaces-and-scope-fix`
- Status: `in-progress`

## Goal

Исправить ложноотрицательную проверку доступа к DigitalOcean и довести Spaces до состояния, пригодного для Terraform state и бэкапов PostgreSQL.

## Проблема

`Infra Inventory` объявил токен непригодным, потому что `GET /v2/account` вернул `403`. Токен при этом полностью работоспособен: читает проекты, дроплеты, ключи, регионы и размеры, и имеет право записи.

Это вторая ошибка одного и того же вида. Первая была с Cloudflare: проверка через `user/tokens/verify` дала `401` на рабочем токене, ограниченном зонами и DNS.

Общий дефект: пригодность токена проверялась эндпоинтом идентичности, а не теми эндпоинтами, которые реально используются. Такая проверка наказывает за корректное сужение прав и провоцирует расширить их, чтобы «починить» диагностику.

## Решение

1. Вердикт `Infra Inventory` опирается на `projects`, `droplets` и пробу записи. `account` остаётся в отчёте как информация, но не влияет на исход.
2. Правило зафиксировано в `docs/integrations/digitalocean.md`, чтобы ошибка не повторилась третий раз.
3. Добавлен workflow `Spaces Configure`: versioning, lifecycle-правила, round-trip проверка.

## Установленные Факты

- токен DigitalOcean достаточен для provisioning, дополнительных прав запрашивать не нужно
- в проекте `simple-vpn-prod` сейчас 0 дроплетов
- bucket доступен и пуст, versioning выключен
- DigitalOcean шифрует Spaces на диске AES-256 по умолчанию и не даёт настройки шифрования на уровне bucket, поэтому `get-bucket-encryption` отвечает ошибкой штатно

## Non-Goals

- не создавать дроплеты
- не создавать дополнительные bucket
- не писать Terraform до подтверждения топологии Business Owner

## Acceptance Criteria

- `Infra Inventory` проходит на текущем токене
- versioning включён, lifecycle-правила применены
- round-trip запись, чтение и удаление подтверждены
- правило проверки доступа записано в durable memory

## Validation

- YAML-валидация, проверка ASCII, проверка встроенного JSON
- dry run `Spaces Configure`, затем apply
- PR loop зелёный
