# Plan: Official APK Download

## 1. Зафиксировать Решение И Живое Состояние

- записать согласованный `site-1` и причины не использовать существующие серверы
- запустить `Infra Inventory`; перед apply потребовать ровно ноль Droplet
- обновить бюджет: существующая Spaces subscription остаётся $5, новый Droplet добавляет $4

## 2. Отделить Постоянные Артефакты От Сайта

- создать отдельный `apk/` namespace в существующем `simple-vpn-infra-backups`
- оставить listing приватным
- публиковать только APK и manifest objects с `public-read`
- не задавать lifecycle expiration для `releases/`
- хранить versioned APK как источник истины, `latest/` как заменяемый указатель

## 3. Сделать Сайт Независимым От Выпуска

- мобильная главная страница сразу показывает основную кнопку загрузки
- версия, размер, SHA-256 и архив читаются из `releases.json`
- устойчивое состояние ошибки не выдаёт неизвестный файл за последнюю версию
- Nginx проксирует `latest`, manifest и versioned downloads из Spaces через официальный домен

## 4. Сделать Постоянный Release Signing

- завести release signing config, включаемый только переменными CI
- хранить PKCS12 и пароли в GitHub Secrets
- собирать `assembleRelease`, затем проверять `apksigner verify --print-certs`
- сохранять fingerprint сертификата в manifest и сверять его с предыдущими версиями

## 5. Автоматизировать Публикацию

- один `Publish APK` workflow для версии, уже записанной в Android-проекте
- concurrency запрещает две публикации одновременно
- до upload проверить монотонность версии, отсутствие versioned object и совпадение signing fingerprint
- после upload versioned object обновить `latest` и manifest
- проверить обе ссылки через `simple-vpn.download`

## 6. Provisioning И DNS

- Terraform создаёт только `site-1` размера `s-1vcpu-512mb-10gb` в `ams3`
- cloud-init ставит Nginx, закрывает password login и публикует статический сайт
- Cloudflare указывает `simple-vpn.download` на `site-1`; домен сайта может быть proxied, в отличие от `vpn-entry`
- HTTPS проверяется снаружи

## 7. PR Loop И Выкладка

- прогнать локальные проверки
- опубликовать feature branch и PR
- дождаться required checks и AI review на PR head
- получить human merge authority
- после merge запустить provisioning, DNS и первую APK publication
- прочитать живой сайт, archive link, latest link, APK signature и hash

## Риски И Ответы

**Release key отсутствует в repository secrets.** Не подменять его debug-ключом. До первой публикации создать или получить постоянный ключ, сохранить recovery copy у Business Owner и только затем положить CI-копию в Secrets.

**Половинная публикация.** Versioned object записывается первым; `latest` и manifest меняются последними. Незавершённый запуск может оставить ещё не объявленный object, но не может направить пользователя на отсутствующий.

**Две публикации спорят за manifest.** Workflow имеет одну concurrency group без отмены активного запуска.

**История случайно удаляется.** У `releases/` нет retention; workflow не имеет delete-команд и отказывает существующему ключу.

**Публичный APK раскрывает backups или listing.** Bucket/listing остаётся private, `public-read` ставится только на выбранные objects под `apk/`; state и backups остаются private.

**Сайт связывается с Control Plane.** `site-1` имеет отдельный адрес; на `simple-vpn.download` нет Control Plane endpoints.

**512 МБ не хватит.** Сервер отдаёт только статические файлы и reverse proxy; APK bytes хранятся в Spaces. Включается swap и лимиты Nginx, а реальная память проверяется после deploy.

## Validation Plan

- `bash sites/official/tools/check-site.sh`
- `bash sites/official/tools/test-publish.sh`
- `terraform -chdir=infra/terraform fmt -check -recursive`
- `terraform -chdir=infra/terraform validate`
- GitHub `Android Build`, `Terraform Check`, required checks, AI Review
- после deploy: HTTPS status, manifest schema, old-version URL, latest URL, SHA-256, certificate fingerprint

## Merge Readiness

Merge-ready означает green required checks, отсутствие конфликтов и blocking findings на текущем head SHA. Production deploy и закрытие стадии происходят только после human merge; локальное или незапушенное состояние `done` не считается.
