# Spec: Исправление передачи адреса в recovery APK-сайта

- Feature ID: `167-apk-site-recovery-output`
- Feature Branch: `feature/167-apk-site-recovery-output`
- Status: `superseded-by-168`

## Goal

Довести recovery существующего `site-1`: шаг перезапуска должен использовать
`SITE_IP`, уже записанный успешным Terraform apply в `GITHUB_ENV`, без повторного
чтения remote backend.

## Incident Fact

Run `33485356292` успешно восстановил state и выполнил apply с результатом
`0 added, 0 changed, 0 destroyed`, но recovery упал до power action: команда
`terraform output` не получила Spaces credentials. Сервер не создавался и не
заменялся.

## Scope

- заменить повторный `terraform output` чтением уже переданного `SITE_IP`;
- сохранить все inventory, no-replacement и conditional-health guards задачи 166.

## Non-Goals

- новый Droplet, resize, rebuild или replacement;
- изменение DNS, Android, Core или APK storage contract.

## Legacy Workspace Assessment

Альтернативной workspace-зависимости нет; одна ветка от актуального `main`.

## Acceptance Criteria

- recovery-step не открывает Terraform backend повторно;
- при недоступном direct health power action отправляется существующему site-1;
- PR checks зелёные; после merge live run возвращает HTTPS-сайт.

## Validation

YAML parse, Terraform Check, Process Baseline, PR Loop Guard, AI Review и live
`Deploy APK Site` после merge.
