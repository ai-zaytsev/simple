# Spec: Диагностика доступа к APK-сайту

- Feature ID: `174-site-access-preflight`
- Feature Branch: `feature/174-site-access-preflight`
- Status: `local-ready`

## Goal

Восстановить штатную доставку responsive content на существующий `site-1`
без создания или пересоздания Droplet. До любых изменений доступа workflow
должен показывать только безопасные fingerprint зарегистрированного site key и
CI deploy key, чтобы исключить очередную догадку о приватном ключе.

## Non-Goals

- новый Droplet, rebuild, resize или замена `site-1`;
- публикация первого APK и изменение release metadata;
- вывод приватных ключей, токенов или адреса origin в логи.

## Scope

- `.github/workflows/deploy-apk-site.yml` и его contract test;
- безопасный одноразовый bootstrap доступа к существующей ноде, только если
  fingerprint подтвердит наличие исходного ключа;
- повторный live deploy и публичная readback-проверка страницы;
- Android, Core, VPN и платёжный контур не меняются.

## Repository Memory Updates

- после live result обновить BO/runbook или tech debt только фактами;
- `spec.md`, `plan.md`, `tasks.md` отражают каждый проверенный шаг.

## Legacy Workspace Assessment

Нет. Работа начата от актуального `main` в одной feature branch и использует
существующий GitHub Actions/PR loop.

## Acceptance Criteria

- workflow показывает fingerprint DO key и deploy key без ключевого материала;
- исходный доступ определяется доказательно;
- CI deploy key добавляется только на существующий `site-1` через временный
  точный `/32`, который обязательно удаляется;
- content deploy проходит, firewall снова web-only, Cloudflare proxy включён;
- публичная страница содержит новую Android/Samsung инструкцию.
- HTML/CSS/JS перепроверяются браузером, а versioned APK сохраняют immutable
  cache policy.

## Validation

- `python .github/scripts/test_deploy_apk_site.py`;
- YAML parse и repository test suite;
- `Process Baseline`, `PR Loop Guard`, `AI Review` на PR head.

## Live Results

- `33504349657`: read-only dry run, единственный `site-1`, Terraform `No
  changes`; зарегистрированный site key доказательно сопоставлен с локальным
  public-key fingerprint;
- `33505335277`: существующий CI public key установлен через точный operator
  `/32`; bootstrap и content SSH правила удалены; adaptive page выложена,
  Cloudflare включён;
- `33506736236`: финальный deploy после исправления cache/TLS; Terraform
  `0 added / 0 changed / 0 destroyed`, Certbot переустановил существующий
  сертификат без renewal, Nginx config valid, content deploy успешен, firewall
  web-only, Cloudflare readback успешен;
- публичный readback: `/` = 200, `/healthz` = 204, `Server: cloudflare`, есть
  `CF-RAY`, HTML содержит Android/Samsung guide, `Cache-Control: no-cache`;
- `/releases.json` = 403, потому что первый официальный APK ещё не опубликован;
  UI штатно показывает отсутствие версии, этот release gate уже записан в
  `docs/release-blockers.md`.

