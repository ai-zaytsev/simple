# Официальная Публикация APK

Официальный адрес: `https://simple-vpn.download`. Сайт и Control Plane намеренно живут на разных доменах и разных адресах.

## Где Что Живёт

| Часть | Место |
| --- | --- |
| HTML, CSS, JavaScript, Nginx | `site-1`, DigitalOcean `ams3`, 1 vCPU / 512 МБ / 10 ГБ |
| Все APK и manifest | DigitalOcean Spaces, prefix `apk/` |
| DNS и внешний proxy | Cloudflare |
| APK signing key | офлайн у Business Owner; рабочая копия в GitHub Secrets |
| Сборка и публикация | GitHub Actions `Publish APK` |

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

1. Изменить `versionName` и увеличить `versionCode` ровно на один в `android/app/build.gradle.kts`.
2. Провести изменение через обычный feature branch, PR, checks, AI review и human merge.
3. Убедиться, что в `docs/release-blockers.md` не осталось открытых блокеров.
4. Запустить `Publish APK` на `main` с `confirm_publish=true`.
5. Прочитать summary: version, SHA-256, latest link и permanent link.
6. Открыть официальный домен и сверить те же значения.

HTML сайта при этом не меняется: он читает manifest. Ручная загрузка через Spaces panel нештатна, потому что обходит проверку подписи, последовательности и неизменяемости.

## Инварианты

- первый upload всегда versioned
- существующий versioned key не перезаписывается
- следующий `versionCode` равен предыдущему плюс один
- certificate fingerprint одинаков у всей истории
- `latest` обновляется только после versioned object
- manifest обновляется последним
- lifecycle не удаляет `apk/releases/`
- workflow не содержит delete operation

Если запуск упал после versioned upload, но до manifest, объект ещё не объявлен пользователям. Повторять тот же номер вслепую нельзя: сначала установить, на каком шаге остановился запуск. Если versioned key уже существует, workflow намеренно откажет.

## Восстановление Сайта

Потеря `site-1` не означает потерю истории: APK живут в Spaces. Новый host создаётся тем же `Deploy APK Site`, DNS переводится на него, а страница снова читает существующий manifest.

Потеря signing key хуже потери всей инфраструктуры: выпустить совместимое обновление станет невозможно. Это максимальный инцидент из [architecture/secrets-model.md](architecture/secrets-model.md), а не задача восстановления сервера.
