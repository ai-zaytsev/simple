# Официальная Публикация APK

Технический контракт подписи, immutable history и publication workflow. Официальный адрес, размещение, текущее состояние и восстановление находятся только в [BO-инструкции](business-owner-operations.md#официальный-apk-канал).

Сайт и Core намеренно живут на разных доменах и адресах. Все APK и manifest хранятся как objects под prefix `apk/`; origin stateless. Signing key имеет офлайн-копию у Business Owner, рабочая копия доступна только release workflow.

## Ссылки

- `/download/latest.apk` — последняя опубликованная версия
- `/download/releases/<version>/simple-vpn-<version>.apk` — постоянная ссылка версии
- `/releases.json` — список версий, который читает главная страница

Все три проходят через официальный домен. Прямой Spaces URL — деталь хранения, а не адрес для пользователя.

## Secrets Для Release Signing

| Secret | Содержание |
| --- | --- |
| `ANDROID_RELEASE_KEYSTORE` | PKCS12, закодированный base64 без переносов |
| `ANDROID_RELEASE_STORE_PASSWORD` | пароль контейнера |
| `ANDROID_RELEASE_KEY_ALIAS` | alias ключа |
| `ANDROID_RELEASE_KEY_PASSWORD` | пароль ключа |

Debug keystore не является fallback. Если release secrets отсутствуют, publication падает до сборки.

Signing key — единственный артефакт, без которого следующая версия не установится поверх предыдущей. До первой публикации Business Owner обязан иметь проверенную офлайн-копию PKCS12 и паролей. GitHub Secrets нельзя прочитать обратно, поэтому они не являются backup.

## Штатный Выпуск

Операторская последовательность и recovery находятся в [BO-инструкции](business-owner-operations.md#официальный-apk-канал). Здесь остаётся только техническое правило: `Publish APK` на `main` собирает release, сначала пишет immutable versioned object, затем `latest` и manifest, публично повторяет hash/signature и только после этого продвигает Core latest. HTML не переделывается при новом релизе. Ручная загрузка через Spaces panel запрещена, потому что обходит проверки последовательности и неизменяемости.

## Инварианты

- первый upload всегда versioned
- существующий versioned key не перезаписывается
- следующий `versionCode` равен предыдущему плюс один
- certificate fingerprint одинаков у всей истории
- `latest` обновляется только после versioned object
- manifest обновляется последним
- Core latest обновляется только после публичной проверки manifest/APK
- публикация latest никогда автоматически не повышает `min_supported_version_code`
- lifecycle не удаляет `apk/releases/`
- workflow не содержит delete operation

Если запуск упал после versioned upload, но до manifest, объект ещё не объявлен пользователям. Повторять тот же номер вслепую нельзя: сначала установить, на каком шаге остановился запуск. Если versioned key уже существует, workflow намеренно откажет.

Если публикация дошла до публичной проверки, но Core latest не обновился, сайт уже считается продвинутым, а workflow — failed. Нельзя публиковать другой APK под тем же номером. Сначала сверяется immutable запись и восстанавливается синхронизация Core по тем же versionCode, URL и SHA-256.

Минимальная поддерживаемая версия управляется отдельно workflow `Application Updates`. Сначала версия публикуется и проходит добровольное обновление, затем при необходимости оператор меняет minimum. Полный runtime contract — [app-updates.md](app-updates.md).

## Восстановление

Операционный порядок восстановления сайта находится в [BO-инструкции](business-owner-operations.md#сайт-site-1). Технический инвариант: потеря stateless origin не удаляет историю objects, а потеря signing key делает совместимое обновление невозможным и является максимальным инцидентом из [secrets-model.md](architecture/secrets-model.md).
