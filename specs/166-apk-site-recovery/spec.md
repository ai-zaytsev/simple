# Spec: Восстановление официального APK-сайта

- Feature ID: `166-apk-site-recovery`
- Feature Branch: `feature/166-apk-site-recovery`
- Status: `local-ready`

## Goal

Вернуть `https://simple-vpn.download/` из Cloudflare 521 в рабочее состояние,
не создавая новый сервер: единственный существующий `site-1` должен быть
безопасно возвращён в Terraform state и восстановлен штатным workflow.

## Incident Facts

- внешний запрос 01.09.2026 получает Cloudflare `521 Web server is down`;
- `Infra Inventory` run `33482581804` видит ровно один Droplet нужного размера
  в `ams3`; Spaces и public-read APK object исправны;
- `Deploy APK Site` run `33482650815` остановился до plan/apply: Droplet есть у
  провайдера, но `digitalocean_droplet.site[0]` отсутствует в remote state;
- новый Droplet Business Owner запретил создавать без отдельного согласования.

## Scope

- безопасное adoption существующего `site-1` и его firewall в remote state;
- отказ при любом неоднозначном inventory или несовпадении имени/региона/size;
- восстановление только существующего origin через power-on/power-cycle, если
  прямой `/healthz` не отвечает;
- обычная проверка DNS, TLS, Cloudflare и публичной страницы после recovery.

## Non-Goals

- создание второго Droplet;
- resize, replacement, rebuild или удаление `site-1`;
- перенос APK из существующего Spaces bucket;
- изменение Android, Core или release format.

## Repository Memory Updates

- `docs/business-owner-operations.md` после live recovery получает итог и run;
- `specs/166-apk-site-recovery/` хранит диагностику, план и фактические шаги.

## Legacy Workspace Assessment

Альтернативной workspace-зависимости нет. Работа идёт в одной feature branch от
актуального `main`; применение выполняется после merge штатным GitHub workflow.

## Acceptance Criteria

- при потерянной state-записи workflow принимает только единственный точный
  `site-1` (`ams3`, `s-1vcpu-512mb-10gb`) и не планирует replacement/delete;
- существующие Droplet и firewall импортируются, а project attachment штатно
  возвращается под управление Terraform;
- recovery не создаёт новый сервер и не меняет одобренный ежемесячный расход;
- `/healthz` и главная страница отвечают через официальный HTTPS-домен;
- PR head проходит required checks, после merge выполнена внешняя live-проверка.

## Validation

- `terraform fmt -check -recursive` и `terraform validate`;
- workflow YAML parse и repository baseline;
- PR Loop Guard и AI Review;
- live `Deploy APK Site` с `apply=true`, `recover_origin=true`;
- независимая проверка страницы браузером и HTTP.

Локально 01.09.2026 проверены YAML parse, обязательные recovery steps и
`git diff --check`. Terraform CLI на рабочей машине отсутствует, поэтому fmt и
validate выполняет обязательный `Process Baseline` на PR head.
