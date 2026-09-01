# Spec: Мобильная версия сайта и установка APK

- Feature ID: `172-mobile-apk-install-site`
- Feature Branch: `feature/172-mobile-apk-install-site`
- Status: `local-ready`

## Goal

Сделать существующую одностраничную страницу `simple-vpn.download` понятным
путём установки APK непосредственно с Android-телефона без отдельной мобильной
копии и без второго источника release metadata.

## Decisions

- один `index.html`, один CSS и один JS обслуживают desktop, phone и tablet;
- адаптация выполняется CSS media queries, а подробности установки раскрываются
  нативным `<details>` на той же странице;
- последняя версия, SHA-256, latest URL и архив продолжают приходить только из
  существующего `/releases.json`;
- сайт получает штатный content-deploy существующего `site-1`: workflow временно
  разрешает SSH только с текущего runner `/32`, атомарно заменяет статические
  файлы и всегда возвращает исходный firewall;
- отдельная нода, домен, мобильный сайт и release feed не создаются.

## Verified Guidance (2026-09-01)

- Android Developers: Android 8.0+ выдаёт разрешение конкретному источнику через
  `Install unknown apps`; Android 7.1.1 и ниже использует общий `Unknown sources`
  в Settings → Security:
  https://developer.android.com/distribute/marketing-tools/alternative-distribution
- Samsung: Settings → Security and privacy → Auto Blocker; Auto Blocker мешает
  установке APK вне Galaxy Store/Google Play и для доверенного APK может быть
  временно выключен:
  https://www.samsung.com/ru/support/mobile-devices/protect-your-galaxy-device-with-the-new-auto-blocker-feature/
- Samsung: разрешение источнику находится в Security and privacy → More security
  settings → Install unknown apps; выбирается браузер, например Chrome:
  https://www.samsung.com/us/support/troubleshoot/TSG10010463/

## Acceptance Criteria

- основная кнопка и номер версии читаются на Android без горизонтального scroll;
- краткий путь установки виден сразу, подробности раскрываются на той же странице;
- есть отдельные простые инструкции для обычного Android, Samsung Auto Blocker и
  Android 7.1.1/старше;
- после установки предлагается вернуть запрет установки из браузера и включить
  Samsung Auto Blocker;
- desktop, 320–360 px phone, современный большой phone и tablet проходят
  визуальную и overflow-проверку;
- релизные данные по-прежнему читаются только из `/releases.json`;
- merged content штатно выкатывается на существующий `site-1`, новый сервер не
  создаётся, firewall после выката остаётся web-only;
- public site после deploy отдаёт новую инструкцию и `/healthz`.

## Known External Gate

Пока первая официальная APK не опубликована, невозможно выполнить финальный
человеческий проход «скачать → разрешить → установить» на телефоне. Вёрстка,
metadata wiring и live content deploy принимаются отдельно; device install
остаётся release gate, а не имитируется тестовым файлом.

## Legacy Workspace Assessment

Одна feature branch от актуального `main`; альтернативная workspace-топология
не требуется.
