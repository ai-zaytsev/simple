# lknpd.nalog.ru — что именно вызывается

Неофициальный API. Здесь записано то, что **проверено по исходнику** MIT-проекта
[`inache-su/moy-nalog-api`](https://github.com/inache-su/moy-nalog-api) — `moy_nalog/client.py`,
`moy_nalog/enums.py`, `moy_nalog/models.py`, — а не восстановлено по памяти.

Записано отдельно от кода по одной причине: у адаптера к чужому недокументированному
API нет второго конца, который упадёт при расхождении. Компилятор про `/income` ничего
не знает. Значит единственное, что отличает проверенное знание от припомненного, —
запись о том, откуда оно взято.

## База

```
https://lknpd.nalog.ru/api/v1
https://lknpd.nalog.ru/api/v2   (только SMS-старт)
```

Заголовки: `Content-Type: application/json`, `Referer: https://lknpd.nalog.ru/`,
`Authorization: Bearer <token>` для авторизованных вызовов.

`deviceInfo` прикладывается к обоим auth-вызовам:

```json
{"sourceType": "WEB", "sourceDeviceId": "<стабильный id>", "appVersion": "1.0.0",
 "metaDetails": {"userAgent": "..."}}
```

`sourceDeviceId` обязан быть стабильным между запусками: это то, что связывает
refresh-token с устройством.

## Вызовы

| что | метод и путь | тело | ответ |
| --- | --- | --- | --- |
| вход по ИНН | `POST /auth/lkfl` | `username`, `password`, `deviceInfo` | `token`, `refreshToken`, `tokenExpireIn`, `profile` |
| обновление токена | `POST /auth/token` | `refreshToken`, `deviceInfo` | `token`, `refreshToken`, `tokenExpireIn` |
| проверка живости | `GET /user` | — | профиль |
| создать чек | `POST /income` | `operationTime`, `requestTime`, `services[]`, `totalAmount`, `client`, `paymentType`, `ignoreMaxTotalIncomeRestriction` | `approvedReceiptUuid` |
| прочитать чек | `GET /receipt/{inn}/{uuid}/json` | — | чек; 404 = нет |
| отменить чек | `POST /cancel` | `operationTime`, `requestTime`, `comment`, `receiptUuid`, `partnerCode` | `approvedReceiptUuid` |

Ссылка на печатный чек собирается, а не приходит:
`https://lknpd.nalog.ru/api/v1/receipt/{inn}/{uuid}/print`.

## Значения, которые нельзя выдумывать

`comment` при отмене — это **русский текст**, не код:

| причина | что уходит в `comment` |
| --- | --- |
| возврат средств | `Возврат средств` |
| чек сформирован ошибочно | `Чек сформирован ошибочно` |

`paymentType`: `CASH` или `WIRE`. Оплата картой физлицом — `CASH`.

`client.incomeType`: `FROM_INDIVIDUAL`, `FROM_LEGAL_ENTITY`, `FROM_FOREIGN_AGENCY`.

Суммы уходят строками: `"399.00"`, не числом.

Время — ISO 8601 **с часовым поясом**, в поясе налогоплательщика.

## Как отличается «сервис лежит» от «мы неправы»

Проект классифицирует временную недоступность по ответу, а не по расписанию:
HTTP 503, либо в сообщении или коде встречается `техническ`+`работ`,
`maintenance`, `service unavailable`, `временно недоступ`.

Это существенно: недоступность ФНС — штатный сценарий, который блокирует продажи
до утра, а неверный запрос — наша ошибка, и лечится она не ожиданием.

`401` означает протухший токен: один refresh и один повтор. `429` — лимит.

## Чего здесь нет

SMS-вход, список доходов, множественные позиции в чеке, профиль. Библиотека это
умеет; нам не нужно, и непереносимое не переносится.
