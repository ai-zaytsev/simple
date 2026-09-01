# Spec: Ключ штатного deploy контента APK-сайта

- Feature ID: `173-apk-site-content-key`
- Feature Branch: `feature/173-apk-site-content-key`
- Status: `local-ready`

## Goal

Исправить mapping SSH key для content deploy существующего `site-1`, выкатить
адаптивную страницу и вернуть Cloudflare proxy после неуспешного run.

## Root Cause

Workflow использовал `${{ secrets.DEPLOY_KEY }}`, которого в repository secrets
нет. Все production SSH workflows получают тот же deploy key из существующего
`${{ secrets.CP_DEPLOY_SSH_KEY }}`. Ошибка произошла до открытия временного SSH,
поэтому firewall и файлы ноды не менялись.

## Scope

- заменить только secret mapping;
- закрепить точное существующее имя contract test;
- после merge повторить `Deploy APK Site` без power recovery;
- подтвердить Terraform no-change, content deploy, firewall restore, Cloudflare
  proxy и responsive live page.

## Non-Goals

Новый secret, сервер, домен, APK publication или изменение release metadata.

## Acceptance Criteria

- content step получает `CP_DEPLOY_SSH_KEY`;
- post-merge run копирует static content на существующий `site-1`;
- временный runner `/32` удалён и firewall снова web-only;
- Cloudflare proxy включён, `/` и `/healthz` доступны;
- live page содержит новую Android/Samsung инструкцию.

## Legacy Workspace Assessment

Минимальная workflow/test/memory правка в одной feature branch от merged main;
альтернативная workspace-топология не требуется.
