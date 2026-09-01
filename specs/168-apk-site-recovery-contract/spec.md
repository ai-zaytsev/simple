# Spec: Контракт передачи адреса APK recovery

- Feature ID: `168-apk-site-recovery-contract`
- Feature Branch: `feature/168-apk-site-recovery-contract`
- Status: `local-ready`

## Goal

Исправить точную перестановку в workflow: `Apply` читает `site_ip` из Terraform
state и публикует `SITE_IP`, а `Recovery` использует это значение без повторного
доступа к backend.

## Evidence

- run `33485881237`: Terraform apply успешен, `0 added / 0 changed / 0 destroyed`;
- следующий shell statement в том же Apply ошибочно читал ещё не созданный
  `SITE_IP` и остановил workflow;
- power action, DNS и новый ресурс не выполнялись.

## Scope

- явно исправить оба соседних шага;
- добавить baseline test, фиксирующий producer/consumer contract.

## Non-Goals

Новый Droplet, replacement, resize, rebuild или изменение topology.

## Legacy Workspace Assessment

Альтернативной workspace-зависимости нет; одна feature branch от merged main.

## Acceptance Criteria

- Apply выполняет `terraform output -raw site_ip` после успешного apply;
- Apply пишет `SITE_IP` в `GITHUB_ENV`;
- Recovery читает `SITE_IP` и не вызывает `terraform output`;
- автоматический test проверяет все три инварианта;
- после merge live recovery возвращает официальный сайт.

## Validation

Новый Python contract test, YAML parse, Terraform Check, Process Baseline,
PR Loop Guard, AI Review и live deploy after merge.
