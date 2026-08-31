# Смена транспорта: итоговое решение

Документ сохраняет техническое обоснование перехода от REALITY с маскировкой под чужие сайты к основному VLESS over WebSocket/TLS за Nginx на собственных доменах. Действующие ноды и их состояние находятся в [BO-инструкции](../business-owner-operations.md).

## Действующая схема

    Android ── TLS ──> Nginx :443
                         ├── обычные пути → самостоятельный cover site
                         ├── edge path    → Core
                         └── secret WS    → Xray / VLESS

Обычный HTTPS probe получает настоящий сайт. VPN-трафик попадает в Xray только при корректном WebSocket upgrade на точном path. Домен, адрес, path и credential приходят в подписанном connection plan.

## Почему REALITY перестал быть основным

Маскировка под чужой домен зависела от внешнего сайта, его сертификата и поведения, которые проект не контролирует. Business Owner потребовал использовать собственные управляемые сайты, а не мимикрировать под сторонний ресурс. Nginx + собственный TLS:

- объясним и проверяем обычными HTTPS-инструментами;
- объединяет cover, VPN и резервный edge на одной расходуемой ноде;
- позволяет менять path/domain/node подписанным планом без APK update;
- делает сертификат и домен критическими зависимостями, которыми теперь управляет проект.

Цена решения: наш сертификат и долгий WebSocket к малому домену легче связать с проектом, чем хороший REALITY cover. Секретность path не является security boundary: пользователь с рабочим планом его знает.

## Финальное решение по доменам и сертификатам

На каждой production-ноде используется apex одного расходного домена, без node subdomain и wildcard. Нода получает собственный certificate через HTTP-01 после установки A-записи. DNS API credentials на ноду не передаются.

Это отличается от раннего проекта «несколько нод на одном домене + wildcard/DNS-01». Тот проект не реализован. Certificate Transparency публикует расходный домен, но не создаёт перечень имён нод внутри него, потому что subdomain на ноду отсутствует.

Порт 80 нужен для ACME и редиректа на HTTPS. Cloudflare proxy на VPN-домене выключен, иначе TLS/WebSocket завершается не на нашей Nginx.

## Требования к Nginx

- только точный WebSocket path с корректным upgrade проксируется в Xray;
- любой другой path получает cover/обычный 404 без VPN banner;
- access log отключён, чтобы не хранить client IP и request path;
- edge path не раскрывает Core fleet и принимает только предусмотренный API traffic;
- TLS certificate, key и Xray configuration имеют ограниченные права;
- config validation выполняется до reload, а health/privacy checks — после;
- остановленный Nginx делает недоступными одновременно cover, VPN и edge, поэтому Node Inspect проверяет все три.

## Изменение connection plan

Transport остаётся конвертом, поэтому UI/session controller не менялись:

| Поле | Основной WebSocket/TLS |
| --- | --- |
| kind | vless-ws-tls |
| endpoint | адрес ноды и 443 |
| TLS identity | server_name нашего расходного домена |
| HTTP routing | host_header и secret path |
| auth | per-device credential UUID |

Поля REALITY не передаются основному transport. Например, flow вместе с WebSocket приводит к отказу handshake, а не игнорируется.

## Запасной REALITY

ADR-024 допускает REALITY как отдельный запасной transport на другом порту. Исторический Test Node устанавливал TCP, но получал EOF до завершения handshake; причина не была доказана. Поэтому REALITY:

- не является действующим доказанным резервом production;
- не включается в оценку доступности основного пути;
- требует отдельной end-to-end стадии перед выдачей клиентам;
- не должен возвращать в архитектуру чужие домены как основной cover.

## Что доказано

Production lifecycle подтвердил:

- собственный публично доверенный certificate;
- cover site по HTTPS;
- VLESS over WebSocket через Nginx;
- node/edge health checks;
- добавление и замена transport params без изменения UI или account model.

Операционные команды, provider placement и live health намеренно не повторяются здесь.
