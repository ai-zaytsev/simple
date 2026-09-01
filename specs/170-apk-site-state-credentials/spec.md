# Spec: Credentials для чтения remote state APK-сайта

- Feature ID: `170-apk-site-state-credentials`
- Feature Branch: `feature/170-apk-site-state-credentials`
- Status: `live-accepted`

## Goal

Сделать inventory guard способным читать Terraform state из приватного Spaces
backend и получить правдивый live verdict без изменения инфраструктуры.

## Root Cause

`terraform init` получал `SPACES_ACCESS_KEY_ID` и `SPACES_SECRET_ACCESS_KEY`, но
следующий step `Confirm live inventory` получал только DigitalOcean token.
Credentials одного GitHub Actions step не наследуются следующим step. Поэтому
`terraform state list` не мог прочитать S3-compatible remote state; stderr
подавлялся, а guard трактовал ошибку как отсутствие ресурса.

## Scope

- передать inventory step существующие Spaces credentials через стандартные
  backend-переменные `AWS_ACCESS_KEY_ID` и `AWS_SECRET_ACCESS_KEY`;
- закрепить обязательность обеих переменных regression test;
- исправить ошибочную feature-memory 169;
- после merge выполнить только read-only dry-run и публичные HTTP-проверки.

## Non-Goals

Terraform apply, power action, новый Droplet, DNS, публикация APK или изменение
секретов.

## Acceptance Criteria

- inventory step получает обе backend credential variables из GitHub Secrets;
- секреты не печатаются;
- post-merge dry-run сообщает `droplets=1`, `recorded_site=true`,
  `adoption=false` и `No changes`;
- публичные `/` и `/healthz` отвечают 200 и 204 соответственно;
- required checks и AI review зелёные.

## Legacy Workspace Assessment

Изменение workflow/test/docs выполняется одной feature branch от актуального
`main`; альтернативная workspace-топология не требуется.

## Live Acceptance

PR #176 merged as `165e240b114156e6512294daaa867369b66e94f2`.
Post-merge read-only run `33500134659` reported exactly one Droplet,
`recorded_site=true`, `adoption=false` and `No changes`. No apply, recovery,
power, DNS or Cloudflare mutation step ran. Public checks returned `/` = 200
and `/healthz` = 204.
