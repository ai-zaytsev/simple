# Tasks: Устойчивое чтение state APK-сайта

- [x] Устранить хрупкий SIGPIPE/pipefail pipeline (live-проверка показала, что это не единственная причина)
- [x] Исправить inventory и reconcile checks
- [x] Расширить regression test
- [x] Обновить BO и feature-memory живым результатом
- [x] Пройти PR loop и получить human merge (PR #175)
- [x] Выполнить post-merge dry-run `33487231323`: защитная проверка осталась ложной (`recorded=false`, `adoption=true`), работа перенесена в feature 170
