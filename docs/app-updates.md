# Управление Обновлениями Приложения

Политика одна для прямого APK и будущего Google Play. Core хранит и подписывает `latest_version_code` и `min_supported_version_code`; канал отвечает только за установку.

## Решение

| Условие | UI | VPN |
| --- | --- | --- |
| `current >= latest` | ничего | работает |
| `min <= current < latest` | «Обновить / Позже» | работает |
| `current < min` | обязательный недismissable диалог | новый plan не выдаётся; клиент также останавливает старт |

Обязательность — следствие сравнения с `min`, а не отдельный флаг. Поэтому Core не может одновременно назвать одну сборку поддерживаемой и потребовать её заменить.

Сравнивается Android `versionCode`. `versionName` показывается человеку и не участвует в порядке: строковое сравнение версий неоднозначно.

## Direct APK

Подписанный config несёт для `direct_apk`:

```json
{
  "url": "https://simple-vpn.download/download/releases/0.2.0/simple-vpn-0.2.0.apk",
  "sha256": "64 lowercase hex characters"
}
```

Порядок на телефоне:

1. проверить подпись и anti-rollback всего remote config;
2. принять только абсолютный HTTPS URL и SHA-256 из 64 hex-символов;
3. проверить, разрешил ли пользователь этому приложению устанавливать пакеты;
4. скачать файл в приватный cache с timeout, redirect и size limits;
5. вычислить SHA-256 во время записи; при несовпадении удалить файл;
6. передать байты в platform `PackageInstaller.Session`;
7. показать системное подтверждение Android и принять status callback.

Storage permission не требуется, файл не становится общедоступным. Android сам дополнительно проверяет application ID, signing certificate и возрастающий versionCode. Эти условия описаны в [официальном Android contract обновлений](https://developer.android.com/google/play/app-updates). Для Android 8+ доверие источнику проверяется через [`canRequestPackageInstalls`](https://developer.android.com/reference/android/content/pm/PackageManager#canRequestPackageInstalls()).

## Google Play Позже

`BuildConfig.UPDATE_CHANNEL` сейчас равен `direct_apk`. Будущий Play build подставит `google_play` и реализацию channel executor. Core policy, verdict, connection plan, аккаунты, VIP и VPN entitlement не меняются.

Материал каждого канала относится только к текущему `latest`. Когда один канал первым продвигает общую версию, артефакты предыдущей версии удаляются из policy; остальные каналы затем штатно прикрепляются к тому же `versionCode`. Поэтому direct APK никогда не получает старый файл, помеченный новой общей версией.

Добровольное предложение показывается только когда текущий канал уже прикреплён к `latest`. Обязательный минимум остаётся общим и показывается даже при временно отсутствующем артефакте: отсутствие файла не может самовольно отменить серверный запрет старой версии.

Google Play предлагает flexible и immediate flows: первый соответствует optional, второй — required. Сам download/install/restart выполняет Play, а решение, какая версия поддерживается, остаётся у Core. Официальное описание flows — [Android Developers: In-app updates](https://developer.android.com/guide/playcore/in-app-updates).

## Операторские Операции

Live-проверка, публикация, изменение minimum и recovery описаны только в [BO-инструкции](business-owner-operations.md#обновление-приложения) и [mini-runbook APK-канала](business-owner-operations.md#официальный-apk-канал).

Технические границы: latest продвигает только `Publish APK` после публичной проверки hash/signature; публикация не меняет minimum. Minimum можно понизить аварийно, при этом подписанный config получает новый возрастающий `seq` и не является replay старого документа.

## Сценарии Отказа

- Config недоступен: используется последний подписанный config; сеть сама по себе не превращается в kill switch.
- APK недоступен/оборван/слишком велик: файл удаляется, показывается повторяемая ошибка.
- Hash не совпал: install session не создаётся.
- Источник не разрешён: открывается системная настройка, затем пользователь повторяет «Обновить».
- Installer отменён или отказал: старая версия остаётся, ошибка показывается после возврата.
- Сборка ниже min пытается обойти UI: `POST /v1/plan` отвечает `426 app_update_required` до выдачи credential/plan.

## Live Acceptance

Текущее состояние версии и наличие channel artifact не фиксируются здесь: их читают `Read The Panel` и `Application Updates / show`, ссылки находятся в BO-инструкции. Критерий end-to-end остаётся техническим: нужны два подписанных выпуска, первый содержит updater, второй проверяет optional и required переходы. Незакрытый проход ведётся в [tech-debt.md](tech-debt.md) и [release-blockers.md](release-blockers.md).
