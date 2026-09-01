# Spec: Закрытие памяти инцидента APK-сайта

- Feature ID: `171-apk-site-incident-closeout`
- Feature Branch: `feature/171-apk-site-incident-closeout`
- Status: `merge-ready`

## Goal

Зафиксировать уже выполненные merge и post-merge live readback восстановления
APK-сайта, чтобы BO-инструкция и feature-memory совпадали с живым состоянием.

## Scope

Только docs/specs. Runtime, Terraform workflow, нода, DNS и Cloudflare не
изменяются. Branch-based flow от merged main; альтернативной workspace
зависимости нет.

## Acceptance Criteria

- feature 170 отмечена `live-accepted`;
- PR #176 и read-only run `33500134659` записаны;
- точный verdict: один Droplet, `recorded_site=true`, `adoption=false`,
  `No changes`, `/` = 200, `/healthz` = 204;
- обнаруженная Node runtime annotation записана как неблокирующий tech debt;
- docs-only PR проходит required checks и требует только human merge.

## Validation

Docs search, `git diff --check`, Process Baseline, PR Loop Guard, AI Review.
