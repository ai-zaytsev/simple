# Tasks: Одна копия проверки здоровья

- [x] Число `204` убрано из `specs/181/spec.md` и `plan.md`
- [x] 181 называет `Rollback Restore` вместо инструкции руками
- [x] `wait-for-core-health.sh` написан
- [x] `deploy-control-plane.yml` зовёт скрипт
- [x] `restore-live.yml` зовёт скрипт
- [x] `rollback-restore.yml` зовёт скрипт и делает checkout
- [x] `check-one-health-check.sh` написан и испытан подделкой
- [x] Guard добавлен в baseline
- [x] Guard упал на первом прогоне (требовал бит исполнения) и исправлен
- [x] `go build` и `go test ./internal/api/` зелёные
