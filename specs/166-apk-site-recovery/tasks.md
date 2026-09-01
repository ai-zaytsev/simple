# Tasks: Восстановление официального APK-сайта

- [x] Подтвердить внешний Cloudflare 521
- [x] Проверить живой DigitalOcean/Spaces inventory run 33482581804
- [x] Доказать потерю state-записи run 33482650815 без создания ресурса
- [x] Зафиксировать отсутствие legacy workspace dependency
- [x] Добавить безопасный adoption существующего site-1
- [x] Добавить условный power-on/power-cycle существующего origin
- [x] Выполнить доступную локальную валидацию; Terraform validate передан обязательному CI
- [x] Пройти PR loop без blocking findings; получить human merge
- [x] Выполнить live recovery без нового Droplet: run 33486380735, power-cycle existing site-1
- [x] Проверить HTTPS: `/` 200, `/healthz` 204; UI корректно показывает отсутствие первого published release
