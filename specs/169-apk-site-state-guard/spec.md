# Spec: Устойчивое чтение state APK-сайта

- Feature ID: `169-apk-site-state-guard`
- Feature Branch: `feature/169-apk-site-state-guard`
- Status: `local-ready`

## Goal

Устранить ложное `recorded_site=false` после успешного восстановления сайта и
закрыть live memory инцидента.

## Root Cause

`terraform state list | grep -q` выполнялся с `set -o pipefail`. После раннего
успешного выхода `grep -q` upstream Terraform мог получить SIGPIPE, и весь
pipeline считался failed. State не терялся; проверка ложно объявляла ресурс
отсутствующим.

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
