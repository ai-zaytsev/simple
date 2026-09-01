# Spec: Устойчивое чтение state APK-сайта

- Feature ID: `169-apk-site-state-guard`
- Feature Branch: `feature/169-apk-site-state-guard`
- Status: `superseded-by-170`

## Goal

Устранить ложное `recorded_site=false` после успешного восстановления сайта и
закрыть live memory инцидента.

## Initial Hypothesis (Disproved Live)

`terraform state list | grep -q` действительно был хрупким при `pipefail`, но
это не было корнем наблюдаемого live verdict. Post-merge dry-run `33487231323`
с безопасным чтением файла по-прежнему получил `recorded_site=false`. Причина:
inventory step не получал credentials Spaces для чтения remote state. Исправление
перенесено в `170-apk-site-state-credentials`.

## Scope

- писать `terraform state list` в файл и проверять файл через `grep -F`;
- закрепить отсутствие опасного pipeline автоматическим тестом;
- обновить BO и feature-memory фактическим live recovery.

## Non-Goals

Live mutation, новый Droplet, power action, DNS или APK publication.

## Legacy Workspace Assessment

Альтернативной workspace-зависимости нет; docs/workflow fix в одной ветке от
актуального merged main.

## Acceptance Criteria

- state guard не использует `terraform ... | grep -q` под pipefail;
- post-merge dry-run сообщает `recorded_site=true`, `adoption=false` и no changes;
- BO-документ показывает рабочий сайт и live run;
- PR checks зелёные.

## Validation

Contract test, Terraform Check, Process Baseline, PR Loop Guard, AI Review и
post-merge dry-run без apply.
