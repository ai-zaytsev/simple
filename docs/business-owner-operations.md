# Система Simple VPN: операционная инструкция Business Owner

Это единственный источник истины для Business Owner о действующей системе: что сейчас развёрнуто, где это находится, где смотреть состояние, как действовать при отказе и что масштабировать. Архитектурные документы объясняют устройство и ограничения для разработчиков, но не дублируют эту инструкцию.

## Как читать состояние

У стабильных фактов и живых чисел разные источники истины:

| Что | Источник истины |
| --- | --- |
| Назначение компонентов, провайдеры, регионы, домены и операции | Этот документ |
| Текущее состояние Core, нод, путей входа, ёмкости, тарифов и версий | [Read The Panel](https://github.com/ai-zaytsev/simple/actions/workflows/panel-check.yml) |
| Ресурсы DigitalOcean и доступность Spaces | [Infra Inventory](https://github.com/ai-zaytsev/simple/actions/workflows/infra-inventory.yml) |
| DNS-зоны, записи и Cloudflare proxy | [DNS Inventory](https://github.com/ai-zaytsev/simple/actions/workflows/dns-inventory.yml) |
| Фактическое состояние конкретной VPN-ноды | [Node Inspect](https://github.com/ai-zaytsev/simple/actions/workflows/node-inspect.yml) |
| Работа отправки почты | [Email Provider Check](https://github.com/ai-zaytsev/simple/actions/workflows/email-provider-check.yml) и [Brevo](https://app.brevo.com/) |
| Платежи тестового магазина | [Payment Acceptance](https://github.com/ai-zaytsev/simple/actions/workflows/payment-acceptance.yml), [личный кабинет ЮKassa](https://yookassa.ru/yooid/signin) и записи Core |
| Сайт, Droplet и Spaces | [DigitalOcean](https://cloud.digitalocean.com/) |
| Счёт и состояние VPN-серверов | [Kamatera](https://console.kamatera.com/login) |
| DNS, proxy и входящая почта | [Cloudflare Dashboard](https://dash.cloudflare.com/) |
| Домены у второго регистратора | [Spaceship account](https://www.spaceship.com/auth/?returnUrl=%2Fapplication%2Fsellerhub%2Fdomains%2F) |
| Состояние Core host | [VDSka](https://old.vdska.ru/page/login) |
| Состояние неиспользуемого `ru` | [RUVDS](https://ruvds.com/) |

Датированный снимок ниже нужен для быстрой ориентации. После его даты живой источник из таблицы выше важнее текста. В публичных Actions-логах адреса серверов и пользователей маскируются; секреты и их значения в эту инструкцию не включаются.

## Состояние на 31 августа 2026 года

Снимок снят в 12:12 МСК запусками `Read The Panel`, `Infra Inventory`, `DNS Inventory` и внешней HTTPS-проверкой.

| Контур | Состояние | Что это означает |
| --- | --- | --- |
| Core | Работает; panel readback и подписанный bootstrap отвечают | Авторизация, планы, конфигурация и управление доступны |
| VPN | 2 из 2 нод на связи, обе `ok`, 0 заблокированных и неисправных | Основной VPN-контур работает |
| Ёмкость VPN | 0 из 1000 соединений сейчас; пик доступной истории 258, или 26% | Запас по измеряемым соединениям есть; перевод этого числа в пользователей пока приблизительный |
| Общий verdict | `WATCH` | Причина одна: остался один готовый расходный домен |
| Публичные пути в Core | Четыре используемых пути отвечали с устройства; один из них в последнем окне имел 88% успеха и помечен `slower` | Следить за следующим окном; `unreachable` или `likely blocked` требует замены пути |
| Пользователи | 1 активный за сутки, 5,1 ГБ за текущий месяц | Это тестовая эксплуатация, не рабочая нагрузка |
| Продажи VIP | Открыты в тестовом контуре; текущая задержка FREE перед покупкой — 1 день | Значение 1 день является серверной тестовой настройкой; продуктовый default — 7 дней |
| Платежи и возвраты | Оплата через adapter ЮKassa развёрнута; возвраты ещё не приняты живой test-store matrix | Нельзя считать денежный контур принятым до успешного deploy refund-изменений и живой сверки оплаты, полного/частичного возврата и VIP |
| Версия приложения | latest `0.1.0 (1)`, minimum `1`, channel artifacts `0` | Политика версий есть, но официального APK ещё не опубликовано |
| Официальный сайт | Работает: `/` = 200, `/healthz` = 204 после recovery run `33486380735` | `site-1` восстановлен power-cycle без создания или замены Droplet; первый официальный APK ещё не опубликован |
| DigitalOcean | Один Droplet размера 1 vCPU / 512 МБ / 10 ГБ в `ams3`; Spaces доступен | Это `site-1`; других Droplet нет |
| Резервные копии | Место в Spaces предусмотрено, но задания PostgreSQL backup и архивации журналов нет | Потеря диска Core сейчас означает потерю данных; это открытый операционный риск |

## Как работает приложение

### Авторизация

Пользователь вводит email. Core создаёт одноразовую magic link со сроком 15 минут и отправляет plain-text письмо через Brevo. Ссылка возвращает пользователя в приложение, после чего Core выдаёт токены конкретной установке. Email хранится только в аккаунтном контуре и не попадает в аналитику или журналы.

Если письмо не приходит, сначала запускается `Email Provider Check`, затем проверяются события Brevo и папка «Спам». Действующие токены и уже поднятые VPN-сессии от сбоя Brevo не зависят; новые входы зависят.

### Получение VPN-конфигурации

Android обращается к Core по `simple-syncbridge.download` либо по одному из подписанных резервных входов. Core выбирает ноды, выдаёт индивидуальный credential и подписывает connection plan. Клиент проверяет подпись и возрастающий номер документа; сам сервер, DNS, маршрут и лимиты он не выбирает.

Последний валидный план хранится на телефоне. При временной недоступности Core установленный клиент продолжает работу в `GRACE`; новая установка без ранее полученного плана подключиться не сможет.

### FREE и VIP

Действующие серверные ограничения видны в `Read The Panel` и важнее старых продуктовых расчётов.

| Возможность | FREE сейчас | VIP сейчас |
| --- | --- | --- |
| Установки приложения | 1 | Без предела |
| Внешние устройства/клиенты | Нет | Без предела |
| Скорость | 10 Мбит/с | Без предела |
| Месячная квота трафика | Не применяется | Не применяется |

Оба тарифа используют одни и те же ноды. VIP — серверное свойство аккаунта, а не флаг Android. Разовая покупка рассчитана на 1, 3 или 12 месяцев; отключение новых продаж не меняет уже действующий VIP.

Любой переход VIP→FREE — окончание оплаченного срока, подтверждённый возврат или operator-команда — в одной транзакции отзывает все внешние credentials. Запись устройства может остаться в истории, но ранее выданная ссылка перестаёт давать VPN-доступ. Единственная разрешённая FREE-установка приложения при этом продолжает работать. Ноды дополнительно сверяют external credential с текущим лимитом тарифа и не принимают его даже при ошибочно оставшемся состоянии `ACTIVE`.

### Подключение VPN

Android поднимает системный `VpnService`, а libXray строит VLESS over WebSocket с TLS к Nginx на выбранной ноде. На том же домене открывается обычный сайт-прикрытие; секретный WebSocket path ведёт в Xray. При отказе primary клиент пробует reserve из подписанного плана и отправляет Core только технический результат подключения.

Ноды не ведут access log и не хранят адреса посещённых сайтов. Расширенный пользовательский trace возможен только после явного нажатия кнопки, ограничен по времени и живёт отдельно от обычной телеметрии.

### Обновление приложения

Core задаёт latest и minimum по `versionCode`. Версия между ними получает добровольное `Обновить / Позже`; версия ниже minimum не получает новый VPN-план. Для direct APK Core передаёт постоянный HTTPS URL и SHA-256, Android проверяет hash и передаёт файл системному установщику. Google Play пока не подключён, но использует ту же серверную latest/min policy.

Первая официальная сборка ещё не опубликована: в Core нет channel artifact, а сайт сейчас в аварии. Штатный выпуск выполняется только workflow `Publish APK`; ручная загрузка файла в Spaces запрещена процессом.

### Восстановление при проблемах

Порядок клиента: текущий Core domain → прямой HTTPS-вход → edge на VPN-нодах → последний валидный план/GRACE. Каждый вход проверяется приложением из пользовательской сети раз в шесть часов и после первого входа. Добавлять новый сервер до проверки существующих входов не нужно.

Глобальная аварийная остановка и minimum version меняются серверно workflow `Service Control` и `Application Updates`. Android не содержит обхода этих решений.

### Удалённая конфигурация

Core без выпуска APK меняет kill switch, ноды и резервы, таймауты, маршрутизацию, лимиты тарифов, доступность продаж, задержку FREE перед покупкой и политику обновлений. Connection plan и remote config подписаны; откат на старую уже применённую конфигурацию блокируется монотонным `seq`.

## Инвентарь инфраструктуры

### Компоненты

| Компонент | Назначение | Провайдер и регион | Сервер/сервис и домен | Зависимости | Последствия отказа |
| --- | --- | --- | --- | --- | --- |
| Core | Auth, планы, remote config, VIP, платежи, телеметрия, панель | VDSka, Финляндия | сервер `fi`; `simple-syncbridge.download`; Go + Nginx | PostgreSQL, DNS Cloudflare, Brevo, ЮKassa, signing material в CI | Новые установки, refresh, покупки и управление не работают; действующий туннель продолжает жить по последнему плану |
| PostgreSQL | Единственный источник истины аккаунтов, нод, планов, метрик и платежей | VDSka, Финляндия | тот же `fi`, локальный сервис | Диск `fi`, Core | Core теряет состояние и перестаёт выполнять основные операции |
| VPN `n-481e` | Пользовательский туннель и edge к Core | Kamatera `EU-ST`, Стокгольм | `simple-vpn-n-481e`; 1 vCPU / 1 ГБ / 20 ГБ; `6015875.xyz` | Kamatera, DNS Spaceship, Nginx, Xray, агенты, Core | Часть пользователей переключается на другую ноду; один публичный edge исчезает |
| VPN `n-c5e6` | Пользовательский туннель и edge к Core | Kamatera `EU-ST`, Стокгольм | `simple-vpn-n-c5e6`; 1 vCPU / 1 ГБ / 20 ГБ; `6047864.xyz` | Kamatera, DNS Cloudflare, Nginx, Xray, агенты, Core | То же; причина прежней остановки Nginx остаётся неизвестной |
| Сайт `site-1` | Главная страница и reverse proxy к APK/manifest в Spaces | DigitalOcean `ams3`, Амстердам | Droplet 1 vCPU / 512 МБ / 10 ГБ; `simple-vpn.download` | DigitalOcean, Cloudflare, Spaces, Nginx, TLS | Нельзя скачать или обновить APK; VPN установленных клиентов не затронут |
| Edge/маскировка | Обычный HTTPS-сайт и скрытый путь к Xray/Core | На обеих Kamatera VPN-нодах, отдельных edge-серверов нет | Домены активных нод | Нода, её сертификат, Nginx, Core | Отказ одного edge уменьшает число путей; отказ всех мешает восстановлению новых клиентов |
| DNS и домены | Стабильный адрес Core, сайт, активные и запасные входы | Cloudflare и Spaceship, глобальные внешние сервисы | таблица доменов ниже | Регистраторы, authoritative DNS, актуальные A/MX/TXT | Ошибка записи может отключить Core, сайт, почту или отдельную ноду |
| Мониторинг | BO-панель, fleet/capacity/connectivity, безопасные журналы | Core на `fi` + GitHub Actions | `/panel` закрыт извне; штатный доступ — `Read The Panel` | Core, PostgreSQL, GitHub Actions | Сервис продолжает работать, но оператор теряет видимость |
| Объектное хранение | Terraform state, место под DB backups, APK и manifest | DigitalOcean Spaces `ams3` | приватный bucket `simple-vpn-infra-backups` | Spaces credentials в CI | State можно восстановить import; APK недоступны; DB recovery сейчас отсутствует |
| Исходящая почта | Magic links и capacity alerts | Brevo, внешний SaaS | sender на `mail.simple-vpn.download` | Brevo, DNS Cloudflare, Core | Новые входы и email-алерты не работают |
| Входящая почта | Приём адресов домена проекта | Cloudflare Email Routing | MX домена `simple-vpn.download` | Cloudflare и внешний конечный ящик | Support/admin письма не доходят; отправка magic links не затронута |
| Платёжный контур | Разовые тестовые платежи и возвраты VIP | ЮKassa test store, внешний SaaS | server API, внешняя checkout page, webhook в Core | Core, PostgreSQL, ЮKassa, DNS Core | Новые оплаты/возвраты не завершаются; VIP не должен меняться без канонического success |
| Канал APK | Сборка, подпись, неизменяемые версии и latest | GitHub Actions + DigitalOcean Spaces + `site-1`/Cloudflare | `https://simple-vpn.download` | signing material в CI, Spaces, сайт, Core update policy | Новые установки/обновления недоступны; установленный APK работает |
| `ru` | Ранее предполагался как RU-probe | RUVDS, Россия | отдельный сервер 1 vCPU / 366 МБ / 9,7 ГБ | Нет runtime-зависимостей | Сейчас на сервис не влияет: роль заменена проверками с Android-устройств |

Отдельных `obs-1`, Prometheus, Grafana, Loki, ClickHouse, WireGuard management network и выделенного PostgreSQL-хоста сейчас нет. Панель и метрики находятся в Core/PostgreSQL; журналы — в journald. SSH к VPN-нодам разрешён только от Core и идёт через него как jump host.

### Домены

| Домен | Регистратор/DNS | Роль и текущее использование |
| --- | --- | --- |
| `simple-syncbridge.download` | Cloudflare / Cloudflare | Стабильный Core; менять IP можно без обновления APK |
| `simple-vpn.download` | Cloudflare / Cloudflare | Публичный APK-сайт, почтовая зона; Cloudflare proxy включён намеренно |
| `6015875.xyz` | Spaceship / Spaceship | Активное прикрытие и edge ноды `n-481e` |
| `6047864.xyz` | Cloudflare / Cloudflare | Активное прикрытие и edge ноды `n-c5e6` |
| `6047865.xyz` | Cloudflare / Cloudflare | Единственный готовый свободный расходный домен |
| `6015874.xyz` | Spaceship / Spaceship | Зарегистрирован, но текущая запись недостижима и требует замены; не считать запасом |

Cloudflare proxy допустим для публичного сайта, но запрещён для доменов VPN-нод: proxy терминирует TLS и ломает туннель. Прямой IP Core также входит в подписанный bootstrap, но намеренно не публикуется в BO-документе; его живость видна как отдельная строка в панели.

## Мини-инструкции по компонентам

### Core `fi`

```
Что это: Go Control Plane за Nginx; единая точка управления.
Где находится: VDSka, Финляндия; simple-syncbridge.download.
Как проверить: Read The Panel; GET /v1/bootstrap должен вернуть 200; Deploy Control Plane проверяет /a, payment return и webhook route.
Как перезапустить: штатно повторить Deploy Control Plane на main; аварийно на консоли fi — systemctl restart simple-vpn-core nginx.
Где смотреть логи: workflow Service Log; приватно на host — journalctl -u simple-vpn-core.
Что делать при отказе: отличить процесс/Nginx от потери host; перезапустить сервисы; если host потерян — поднять замену и перевести A-запись стабильного домена.
Как восстановить: бинарь и схема берутся из main через Deploy Control Plane. Данные восстановить сейчас неоткуда, пока не внедрён проверенный PostgreSQL backup.
```

### PostgreSQL на `fi`

```
Что это: единственная база аккаунтов, entitlement, fleet, config, метрик и платежей.
Где находится: локально на fi, VDSka, Финляндия.
Как проверить: Read The Panel должен полностью сформироваться; на host — systemctl is-active postgresql и psql по локальному DSN.
Как перезапустить: systemctl restart postgresql, затем simple-vpn-core.
Где смотреть логи: journalctl -u postgresql; ошибки Core — Service Log.
Что делать при отказе: проверить диск, память, PostgreSQL и только затем Core; не очищать WAL/таблицы вручную.
Как восстановить: сейчас полного пути нет — backup job и restore drill отсутствуют. До их появления сохранять диск/снимок host и не выполнять разрушительные действия.
```

### VPN-нода `n-481e`

```
Что это: VLESS/WebSocket/TLS VPN, сайт-прикрытие и HTTPS edge в Core.
Где находится: Kamatera EU-ST, Стокгольм; 6015875.xyz.
Как проверить: Node Inspect с alias n-481e и Read The Panel; ожидаются ok, выдаётся да, nginx/xray/metrics active, TLS и точный edge отвечают.
Как перезапустить: Update Node восстанавливает и проверяет сервисы; полный reboot — только через Kamatera Console, затем Node Inspect.
Где смотреть логи: Node Inspect; прямой journal доступен только через fi. Не включать access/debug logging.
Что делать при отказе: перестать выдавать через Lifecycle, проверить второй reserve; при блокировке не чинить IP, а создать замену Add Server на готовом домене.
Как восстановить: нода не восстанавливается из диска — Add Server создаёт новую, затем старая выводится Retire Node.
```

### VPN-нода `n-c5e6`

```
Что это: VLESS/WebSocket/TLS VPN, сайт-прикрытие и HTTPS edge в Core.
Где находится: Kamatera EU-ST, Стокгольм; 6047864.xyz.
Как проверить: Node Inspect с alias n-c5e6 и Read The Panel; критерии те же, что у n-481e.
Как перезапустить: Update Node; полный reboot — Kamatera Console, затем Node Inspect.
Где смотреть логи: Node Inspect и journald через jump host fi.
Что делать при отказе: если SSH/метрики живы, а edge нет — проверить nginx первым; именно такой отказ уже происходил. При блокировке заменить ноду/IP.
Как восстановить: создать новую ноду штатным Add Server, проверить с пользовательской сети, затем Retire Node для старой.
```

### Сайт `site-1`

```
Что это: Nginx с главной страницей и proxy к публичным APK-объектам Spaces.
Где находится: DigitalOcean ams3; simple-vpn.download за Cloudflare.
Как проверить: / и /healthz должны отвечать 200 через публичный домен; Infra Inventory должен видеть ровно один Droplet нужного размера.
Как перезапустить: Deploy APK Site с `apply=true`, `recover_origin=true`: workflow проверяет прямой /healthz и только при отказе делает power-on/power-cycle существующего site-1. Для ручной диагностики: DigitalOcean → site-1 → Web Console → systemctl restart nginx. Публичный SSH закрыт firewall.
Где смотреть логи: journalctl -u nginx и nginx error log через Web Console; access log не нужен.
Что делать при отказе: 521 означает, что Cloudflare не достигает origin — запустить Infra Inventory, затем Deploy APK Site recovery. Если Droplet существует, но отсутствует в Terraform state, workflow принимает только единственный точный site-1 в ams3 нужного размера; любой другой inventory останавливает операцию. Не переключать DNS на Core.
Как восстановить: существующий host возвращается в state и перезапускается тем же workflow без нового Droplet. Если provider действительно показывает ноль Droplet, остановиться: создание нового сервера требует подтверждения Business Owner. История APK находится не на Droplet, а в Spaces.
```

### DNS и домены

```
Что это: стабильный адрес Core, публичный сайт, почта и расходные прикрытия нод.
Где находится: Cloudflare и Spaceship; регистрация и DNS совпадают по таблице выше.
Как проверить: DNS Inventory; для активных VPN-доменов A-запись одна и proxied=0, для сайта proxy допустим.
Как перезапустить: неприменимо; при сбое провайдера используются его status/support и резерв у второго регистратора.
Где смотреть логи: summary DNS Inventory и audit log провайдера.
Что делать при отказе: не менять всё сразу; определить роль домена. Core переводится сменой A, нода — заменой ноды, сайт — восстановлением site-1.
Как восстановить: Point DNS или соответствующий deploy workflow; затем обязательная публичная TLS-проверка и Read The Panel.
```

### Мониторинг

```
Что это: /panel и admin overview Core плюс безопасные диагностические workflows.
Где находится: данные в PostgreSQL на fi, UI в Core, запуск/вывод в GitHub Actions.
Как проверить: Read The Panel должен завершиться success и показать свежий generated_at.
Как перезапустить: отдельного сервиса нет; перезапускается Core/PostgreSQL.
Где смотреть логи: Actions run, Service Log, Node Inspect.
Что делать при отказе: если VPN работает, считать это потерей видимости; восстановить Core/DB до любых изменений fleet вслепую.
Как восстановить: deploy Core из main; история мониторинга зависит от той же PostgreSQL и сейчас не имеет backup.
```

### DigitalOcean Spaces

```
Что это: один приватный bucket для Terraform state, будущих DB backups и официальных APK.
Где находится: DigitalOcean ams3, simple-vpn-infra-backups.
Как проверить: Infra Inventory — bucket reachable, public-read для отдельного probe-object работает, listing остаётся приватным.
Как перезапустить: неприменимо; проверить status DigitalOcean и доступ CI.
Где смотреть логи: Infra Inventory, Spaces Configure и конкретный release workflow.
Что делать при отказе: не публиковать новый APK и не запускать Terraform apply без доступного state; существующий VPN продолжает работать.
Как восстановить: Terraform state допускает import; APK перепубликовываются только из доверенного signed artifact. PostgreSQL backup пока отсутствует.
```

### Почта

```
Что это: Brevo отправляет magic links и алерты; Cloudflare Email Routing принимает входящую почту домена.
Где находится: внешние SaaS; DNS mail.simple-vpn.download в Cloudflare.
Как проверить: Email Provider Check; нужны accepted и delivered для gmail/yandex, затем ручная проверка Inbox/Spam и неизменённой ссылки.
Как перезапустить: неприменимо; при смене credential обновить GitHub Secrets и повторить Deploy Control Plane.
Где смотреть логи: Brevo transactional logs и summary workflow; адреса получателей в публичный лог не выводить.
Что делать при отказе: повторить probe, проверить кредиты/репутацию/DNS; не трогать работающие VPN-сессии. Резервного отправителя сейчас нет.
Как восстановить: восстановить Brevo/DNS или подключить заранее подготовленный provider adapter; входящие письма восстанавливаются в Cloudflare Email Routing.
```

### ЮKassa

```
Что это: первый adapter общего платёжного модуля, сейчас test store, разовые VIP-платежи и возвраты через провайдера исходного платежа.
Где находится: внешний сервис ЮKassa; checkout у провайдера, webhook и entitlement в Core.
Как проверить: Payment Acceptance сопоставляет payment/refund rows с authenticated GET ЮKassa. Payment Webhook Replay проверяет повторные события, Refund Lost Response — exact POST retry с исходным key в пределах 24 часов; оба скрывают provider IDs и доказывают неизменность денег/VIP. Затем сверить tier в Read The Panel и credentials в Device Access/Service Log. Возврат браузера или ответ POST сам по себе не является успехом.
Как перезапустить: неприменимо; при сомнении закрыть новые продажи workflow Purchases, не меняя существующий VIP.
Где смотреть логи: операции ЮKassa и отфильтрованный Service Log; секреты и provider error body не печатать.
Что делать при отказе: оставить платёж/refund незавершённым, VIP не активировать и не прекращать; после восстановления перечитать канонический status server API.
Как восстановить: повторить idempotent server operation или webhook. Для потерянного refund сначала найти операцию по исходному payment в ЮKassa; после 24 часов не создавать её повторно вслепую. Менять провайдера только для новых платежей после завершения взаиморасчётов старого.
```

31 августа — 1 сентября 2026 года живая test-store матрица завершена. Full refund вернул 399 ₽ и отозвал VIP/external access (ноды `2→1`, сохранённая ссылка перестала работать). Partial refund после 8 суток вернул рассчитанные Core `222,01 ₽`; DB/provider дали `succeeded/succeeded`, VIP прекратился. Четыре повтора webhook получили `HTTP 200 / applied=false`; exact refund POST retry вернул тот же provider object, операций осталось ровно одна. Брошенный checkout самостоятельно перешёл `canceled/canceled`, VIP не выдавался. После тестов продажи открыты, FREE-период 7 дней, панель `FREE=1`, `VIP=0`. Невоспроизводимым остался только refund `insufficient_funds`: ЮKassa не публикует для test store детерминированный триггер, поэтому успех этого отрицательного сценария не имитировался.

### Официальный APK-канал

```
Что это: Publish APK собирает и подписывает APK, пишет immutable version, latest и manifest, затем синхронизирует Core.
Где находится: GitHub Actions, Spaces apk/, simple-vpn.download.
Как проверить: latest.apk и постоянная ссылка должны скачиваться через официальный домен; workflow повторно проверяет SHA-256 и signing certificate.
Как перезапустить: сайт — по инструкции site-1; неудачный publish повторяется только после определения последнего завершённого шага.
Где смотреть логи: Publish APK summary и Application Updates / show.
Что делать при отказе: не перезаписывать существующий versioned key и не выпускать другой APK под тем же versionCode.
Как восстановить: пересоздать site-1 поверх сохранённой истории Spaces; signing key восстанавливается только из офлайн-копии Business Owner.
```

## Мониторинг и пороги вмешательства

### Главная панель

`Read The Panel` — первая проверка при любом инциденте и после каждого deploy. Значения трактуются так:

| Показатель | Норма | Требует вмешательства |
| --- | --- | --- |
| Capacity verdict | `NORMAL`; `WATCH` означает наблюдать причину | `SCALE_REQUIRED` — начать расширение; `CRITICAL` — действовать немедленно |
| Недельный peak/capacity | <60% | 60–79% `WATCH`; 80–94% масштабировать; ≥95% критично |
| P95 utilisation | <60% | 60–79% наблюдать; ≥80% масштабировать |
| Запасные ноды | ≥1 | 0 — масштабировать до следующего отказа |
| Свободные расходные домены | ≥2 | 1 — `WATCH`; 0 — новую ноду поднять нельзя |
| Node verdict | `ok`; last sample моложе 3 минут | `silent`; loss ≥5%; latency ≥250 мс; CPU ≥85%; memory ≥90%; load1 ≥4 |
| Endpoint verdict | `works` с device checks | `slower` — наблюдать окно; `likely blocked`/`unreachable` — вывести и заменить |
| Public Core | `/v1/bootstrap` = 200 | Любой другой ответ или timeout |
| APK site | `/healthz` = 200 | 5xx/521/timeout; обновления недоступны |
| Email | provider accepted + delivered, письмо видно человеку | exhausted, bounce/blocked, нет письма или ссылка переписана |
| Update policy | latest/min совпадают с опубликованным релизом; artifact есть после первой публикации | min > latest, отсутствует artifact у опубликованного latest, hash/signature mismatch |

Панель не показывает ресурсные метрики самого `fi`, PostgreSQL и `site-1`. До появления host monitoring их нужно проверять в provider console/на host. Для `fi` вмешательство начинается при свободном диске <3 ГБ, доступной памяти <200 МБ или средней CPU >50% сутки. Для `site-1` любой 5xx уже является вмешательством: его единственная работа слишком мала, чтобы высокая нагрузка считалась нормой.

### Остальные панели

| Панель | Для чего | Что смотреть |
| --- | --- | --- |
| [GitHub Actions](https://github.com/ai-zaytsev/simple/actions) | Все штатные изменения и диагностика | Последний run на `main`, conclusion, summary и конкретный head SHA |
| [Kamatera Console](https://console.kamatera.com/login) | Фактические машины и биллинг VPN | Ровно две активные production-ноды сейчас; power, traffic, счёт; забытых машин быть не должно |
| [DigitalOcean](https://cloud.digitalocean.com/) | `site-1`, firewall, Spaces и расход | Ровно один Droplet сейчас; регион/size; Spaces; неожиданные ресурсы |
| [Cloudflare](https://dash.cloudflare.com/) | DNS, proxy сайта, Email Routing | Активные зоны, A/MX/TXT, proxy только у сайта, ошибки origin |
| [Spaceship](https://www.spaceship.com/auth/?returnUrl=%2Fapplication%2Fsellerhub%2Fdomains%2F) | Два расходных домена | A-записи, срок регистрации, доступ API |
| [Brevo](https://app.brevo.com/) | Отправка magic link | Кредиты, delivery/bounce/blocked, репутация sender |
| [ЮKassa](https://yookassa.ru/yooid/signin) | Test payments/refunds | status, сумма, исходный способ и webhook delivery; не считать return page или ответ POST подтверждением |
| [VDSka](https://old.vdska.ru/page/login) | `fi` | power, диск, сеть, доступ консоли |
| [RUVDS](https://ruvds.com/) | Неиспользуемый `ru` | Только расходы и power; сервис от него не зависит |

## Типовые отказы: короткий порядок

| Симптом | Первая проверка | Действие |
| --- | --- | --- |
| Новые пользователи не могут войти | Core bootstrap, затем Email Provider Check | Восстановить Core или Brevo; существующий VPN не отключать |
| VPN не подключается у части пользователей | Endpoint и node verdict в панели | Если один node/edge плох — Lifecycle/замена; если один сегмент — не выводить весь fleet |
| VPN не подключается у всех | Core plan доступен? обе ноды? kill switch/minimum? | Сначала отменить ошибочную policy, затем восстановить/заменить ноды |
| Сайт/скачивание не работает | `/healthz`, Cloudflare error, Infra Inventory | При 521 запустить Deploy APK Site с recovery; Web Console использовать для оставшейся диагностики |
| Оплата вернулась, VIP не появился | Canonical payment status и webhook | Не активировать вручную по странице; восстановить webhook/API и повторить безопасную обработку |
| Аккаунт уже FREE, но старая внешняя ссылка подключается | Tier аккаунта и число credentials в Service Log | Считать access-control инцидентом: закрыть новые продажи, проверить Core deploy и node sync; для FREE нода должна получать только app credential |
| Панель недоступна, VPN работает | Core/DB и Read The Panel | Считать потерей видимости; восстановить мониторинг до изменений fleet |
| Заканчивается диск `fi` | Disk/WAL/PostgreSQL | Остановить необязательный рост, расширить диск, затем проверить Core и DB |
| Домен ноды заблокирован | Device checks плохие, Core checks хорошие | Создать ноду с новым IP/доменом, затем вывести старую |

## Рост нагрузки

Автомасштабирование и автоматическое увеличение платных ресурсов запрещены. Любой новый постоянный сервер сверх текущего инвентаря — отдельное решение Business Owner после измерения.

### Растёт число пользователей

```
Как понять, что достигнут лимит: растут active users, Brevo consumption, payment volume, Core disk/memory и VPN P95; одного числа регистраций недостаточно.
Что масштабировать: сначала узкое место — почтовый лимит, Core/DB или VPN, а не всё сразу.
Как это сделать: увеличить соответствующий тариф SaaS; расширить диск/RAM fi; добавить VPN-ноду только по capacity verdict.
Что проверить после: magic link на двух почтовых классах, Read The Panel, Core logs, реальное подключение и provider bill.
```

### Растёт число VPN-соединений

```
Как понять, что достигнут лимит: peak или P95 ≥80%, запасных нод 0, либо SCALE_REQUIRED.
Что масштабировать: пул Kamatera и запас расходных доменов.
Как это сделать: сначала иметь ≥2 готовых домена, затем Add Server в измеренном EU-ST; не клонировать конфигурацию вручную.
Что проверить после: Node Inspect, privacy check, TLS/edge, device check из России, Read The Panel и наличие хотя бы одной spare node.
```

### Растёт трафик

```
Как понять, что достигнут лимит: растут Мбит/с и месячный объём в панели, provider traffic/bill приближается к пакету, появляются loss/latency.
Что масштабировать: полосу/число VPN-нод или серверную продуктовую политику; не сайт и не Core.
Как это сделать: добавить ноду, изменить размер/полосу у провайдера либо отдельно утвердить и реализовать traffic quota.
Что проверить после: provider bill, traffic split, loss/latency, скорость FREE 10 Мбит/с и отсутствие ограничения VIP.
```

### Растёт нагрузка на Core

```
Как понять, что достигнут лимит: CPU fi >50% сутки, свободная память <200 МБ, растут latency/error в Core.
Что масштабировать: сначала RAM/диск текущего host; затем перенести Core на более мощный host.
Как это сделать: подготовить host с Nginx/Core и теми же CI-managed secrets, проверить локально, затем сменить A simple-syncbridge.download.
Что проверить после: /a, bootstrap, plan, webhook route, Read The Panel и вход/подключение реального устройства без APK update.
```

### Растёт нагрузка на PostgreSQL

```
Как понять, что достигнут лимит: свободный диск <3 ГБ, память fi <200 МБ, WAL/таблицы быстро растут, запросы панели/Core замедляются.
Что масштабировать: диск, retention/индексы; после измерения — отдельный DB host.
Как это сделать: сначала создать и проверить backup/restore, затем перенести базу и сменить только серверный DSN.
Что проверить после: migrations, auth, plan seq, entitlement, payments, panel history и тестовое восстановление.
```

### Растёт нагрузка на отдельную VPN-ноду

```
Как понять, что достигнут лимит: CPU ≥85%, memory ≥90%, load1 ≥4, loss ≥5%, latency ≥250 мс или её доля соединений непропорциональна.
Что масштабировать: добавить ноду и перераспределить выдачу; блокированную ноду заменить, а не resize.
Как это сделать: Add Server → Node Inspect → serving; старую при необходимости draining → Retire Node.
Что проверить после: обе точки обзора, пользовательское подключение, privacy check, capacity/spares и отсутствие оставленной платной машины.
```

## Известные операционные пробелы

Это не скрытые допущения, а границы текущей системы:

- нет автоматического PostgreSQL backup и проверенного restore;
- нет выгрузки journald в Spaces, несмотря на подготовленный namespace;
- нет host monitoring для `fi`, PostgreSQL и `site-1`;
- нет резервного email-провайдера;
- из двух зарегистрированных свободных доменов только один сейчас пригоден;
- official APK не опубликован, direct update end-to-end не принят;
- test-store ЮKassa не пройден полностью на живом APK для оплаты, полного/частичного возврата и VIP readback;
- месячная traffic quota FREE не реализована, хотя старые расчёты использовали 20 ГиБ;

Условия закрытия и риск каждого долгоживущего пункта ведутся в [tech-debt.md](tech-debt.md). Блокирующие выпуск пункты ведутся отдельно в [release-blockers.md](release-blockers.md).
