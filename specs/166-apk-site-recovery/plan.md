# Plan: Восстановление официального APK-сайта

1. Расширить inventory guard: разрешить adoption только единственного точного
   `site-1`, сохранив отказ для любого другого live inventory.
2. Импортировать существующие Droplet/firewall перед plan; не импортируемый
   project attachment Terraform создаёт повторно как декларативное назначение.
3. Запретить replacement/delete и игнорировать непрочитываемый после создания
   cloud-init `user_data` у принятого Droplet.
4. При явном `recover_origin=true` проверить origin напрямую и только при отказе
   выполнить power-on либо power-cycle существующего Droplet.
5. Пройти PR loop, после merge применить recovery и проверить публичный сайт.

## Risks

- принятие чужого Droplet: закрыто строгой проверкой count/name/region/size;
- скрытая замена при import drift: `prevent_destroy` и JSON plan запрещают delete;
- лишняя перезагрузка: action вызывается только после неуспешного direct health;
- потеря state снова: live closeout должен подтвердить state `known=true` повторным
  dry-run после восстановления.
