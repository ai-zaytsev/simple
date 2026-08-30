# Tasks: Ограниченный пользовательский destination trace

- [x] Создать feature branch от актуального `main`
- [x] Записать уточнённое решение Business Owner и non-goals
- [x] Зафиксировать отсутствие legacy workspace dependency
- [x] Выбрать implementation agent и подготовить worker context
- [x] Добавить тестируемую модель ограниченного окна записи
- [x] Сделать повторный start идемпотентным без изменения deadline
- [x] Восстанавливать готовый trace после повторного открытия приложения
- [x] Добавить регрессионные unit tests
- [x] Обновить privacy model и Android architecture
- [x] Удалить устаревший release blocker
- [x] Выполнить локальную валидацию: `git diff --check`, caller audit и проверка двух оставшихся open blockers; Android toolchain отсутствует локально, unit/build переданы штатному GitHub Android Build
- [x] Запушить feature branch и открыть PR №157; сбой первого PR в `Publish-FeaturePR.ps1` записан в `docs/tech-debt.md`, PR создан вручную с корректным body
- [x] Получить green checks и documented AI review fallback; внешний reviewer `claude` выбран, но `AI_REVIEW_COMMAND` не настроен, поэтому review authority остаётся у человека
- [ ] Получить human review и merge authority
