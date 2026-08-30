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

Workflow `Application Updates`:

- `show` — читает live latest/min и число настроенных channel artifacts;
- `set-minimum` — задаёт `min` в пределах `1..latest` без APK update.

`min` можно понизить: это аварийное восстановление поддержки старой сборки. Signed config получает новый больший `seq`, поэтому intentional rollback policy не является replay старого документа.

Latest нельзя вводить вручную. Его продвигает только `Publish APK` после того, как workflow:

1. записал immutable versioned object;
2. обновил `latest` и manifest;
3. скачал APK через официальный домен;
4. повторно сверил SHA-256 и release signature;
5. передал Core постоянный versioned URL и hash.

Если публичная публикация успела завершиться, а синхронизация Core временно не удалась, повтор `Publish APK` распознаёт точную уже опубликованную пару `versionName/versionCode`, не перезаписывает immutable APK, заново проверяет публичный hash и подпись и повторяет только продвижение Core.

Публикация не меняет `min`, поэтому новая версия сначала всегда добровольная. Принудительной она становится только отдельной operator-операцией после проверки.

## Сценарии Отказа

- Config недоступен: используется последний подписанный config; сеть сама по себе не превращается в kill switch.
- APK недоступен/оборван/слишком велик: файл удаляется, показывается повторяемая ошибка.
- Hash не совпал: install session не создаётся.
- Источник не разрешён: открывается системная настройка, затем пользователь повторяет «Обновить».
- Installer отменён или отказал: старая версия остаётся, ошибка показывается после возврата.
- Сборка ниже min пытается обойти UI: `POST /v1/plan` отвечает `426 app_update_required` до выдачи credential/plan.

## Live Acceptance

После deploy читаются `Read The Panel` и `Application Updates / show`: ожидаются latest `0.1.0 (1)`, minimum `1`; channel material появится после первой официальной публикации.

End-to-end direct APK требует двух подписанных выпусков: первая опубликованная сборка должна уже содержать updater, следующая проверяет optional/required переход. Пока общие release blockers открыты, этот проход не считается выполненным и записан в [tech-debt.md](tech-debt.md).
