# Spec: Official APK Download

## Решение Business Owner

29.08.2026 согласовано отдельное размещение официального сайта:

- хост `site-1`
- DigitalOcean, регион `ams3`
- `s-1vcpu-512mb-10gb`: 1 shared vCPU, 512 МБ RAM, 10 ГБ SSD
- APK хранятся в DigitalOcean Spaces
- официальный домен: `simple-vpn.download`

Перед решением рассмотрено размещение на существующих серверах. Оно отвергнуто:

- `fi` уже несёт Control Plane, PostgreSQL и WireGuard; рекламируемый сайт на том же адресе связал бы блокировку сайта с Control Plane
- `ru` имеет 366 МБ RAM и по архитектуре не принимает публичный трафик
- VPN-ноды расходуемые, поэтому не могут владеть постоянной историей релизов

Business Owner отдельно уточнил, что перед этой задачей в DigitalOcean не должно быть ни одного Droplet: `site-1` будет первым. Это проверяется живой инвентаризацией до apply, а не принимается по старому снимку документации.

## Бизнес-Результат

`simple-vpn.download` становится официальным каналом установки Android-приложения. Главная страница ведёт на последнюю опубликованную подписанную APK, а каждая выпущенная после запуска сайта версия остаётся доступна по постоянной отдельной ссылке.

## Контракт Публикации

- APK собирается только в GitHub Actions и подписывается постоянным release-ключом
- публикация принимает версию из Android-проекта, проверяет подпись и SHA-256 до загрузки
- `versionCode` строго растёт, `versionName` не повторяется
- версия публикуется в новый неизменяемый ключ `releases/<version>/simple-vpn-<version>.apk`; существующий ключ не перезаписывается
- только после успешной загрузки версии атомарно обновляются `latest/simple-vpn.apk` и `releases.json`
- у APK-архива нет lifecycle-правила удаления
- сайт читает `releases.json`, поэтому новая версия не требует изменения HTML, CSS, JavaScript или Nginx
- `/download/latest.apk` всегда отдаёт объект `latest/simple-vpn.apk`
- отдельная ссылка сохранённой версии проходит через официальный домен

## Scope

- статический мобильный сайт официальной загрузки и его проверка
- release signing configuration Android без ключей в репозитории
- штатный workflow публикации APK
- отдельный `apk/` namespace в Spaces при приватном bucket listing
- Terraform и cloud-init для постоянного `site-1`
- Cloudflare DNS-запись для `simple-vpn.download`
- durable memory о размещении, выпуске и эксплуатации сайта

## Non-Goals

- Control Plane endpoints на `simple-vpn.download`
- Google Play, Firebase App Distribution или автообновление установленного приложения
- удаление, переименование или перезапись уже опубликованной версии
- хранение APK на диске `fi`, `ru` или VPN-ноды
- публикация debug APK как официального релиза
- альтернативная workspace-топология

## Секреты

- release keystore и его пароли живут только в GitHub Secrets
- Spaces и Cloudflare credentials переиспользуют существующие repository secrets
- ни один приватный ключ, пароль, access token или адрес хоста не попадает в repository, Terraform state или публичный Actions log

## Acceptance Criteria

- решение о `site-1` и его обоснование зафиксированы в durable memory
- перед apply живая инвентаризация показывает ноль DigitalOcean Droplet
- Terraform plan создаёт ровно один `site-1` нужного размера в `ams3` и остаётся в одобренном бюджете
- `https://simple-vpn.download/` отвечает и показывает последнюю опубликованную версию
- `https://simple-vpn.download/download/latest.apk` отдаёт подписанную APK
- версия доступна по постоянной ссылке через официальный домен
- публикация второй тестовой фикстурой доказывает, что первая ссылка не исчезает, а `latest` переключается вперёд
- production workflow отказывает неподписанной, повторной или немонотонной версии
- APK history в Spaces не имеет автоматического удаления
- branch опубликована, PR существует, required checks зелёные, blocking findings отсутствуют и merge authority остаётся у человека
- после production deploy официальный домен и опубликованная APK прочитаны снаружи и результат сверки назван

## Repository Memory Updates

- `docs/architecture/mvp-topology.md`: размещение и бюджет `site-1`
- `docs/integrations/digitalocean.md`: постоянный сайт-хост
- `docs/integrations/dns.md`: живая запись домена сайта
- `docs/architecture/android-client.md`: release signing и публикация
- `docs/release-apk.md`: штатная процедура выпуска и восстановления
- `specs/150-official-apk-download/{spec,plan,tasks}.md`: фактический ход стадии

## Legacy Workspace Assessment

Проверено по `AGENTS.md`, `CLAUDE.md`, `docs/worker-orchestration.md` и scripts process-layer: отдельная workspace-механика не требуется. Работа идёт в `feature/150-official-apk-download`, созданной от актуального `main`, и завершается одним PR.

## Validation

- статические проверки HTML, CSS, JavaScript и release manifest
- тест публикационного алгоритма на временном локальном S3-подобном представлении без реальных APK
- Android unit tests и release assemble в GitHub Actions
- `terraform fmt`, `init`, `validate`, `plan`
- cloud-init syntax и Nginx configuration checks
- PR required checks и AI review на текущем PR head SHA
- внешний HTTPS download, SHA-256 и `apksigner verify` для опубликованного APK
