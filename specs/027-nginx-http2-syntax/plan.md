# Plan: Nginx HTTP2 Syntax

- Feature ID: `027-nginx-http2-syntax`
- Feature Branch: `feature/027-nginx-http2-syntax`
- Owner: `orchestrator`

## Implementation Slices

1. Включить HTTP/2 параметром `listen`.
2. Публиковать вывод `nginx -t` при отказе.
3. Прогон.

## Risks

- при переходе на nginx 1.25 и новее параметр `http2` в `listen` объявлен устаревшим: он продолжает работать, но при смене образа стоит вернуться к отдельной директиве

## Validation Plan

- локальный рендер
- прогон с уборкой

## Merge Readiness

Закрывается прогоном, подтвердившим оба транспорта.
