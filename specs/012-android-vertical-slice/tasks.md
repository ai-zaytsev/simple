# Tasks: Android VPN Vertical Slice

- [x] Создать и заполнить `spec.md`
- [x] Создать и заполнить `plan.md`
- [x] Каркас Gradle с закреплёнными версиями
- [x] `VpnConnectionState`, `VpnController`, `SimpleVpnService`
- [x] `TunConfigurator`: защита от петли и от утечки IPv6
- [x] `NetworkMonitor`: смена Wi-Fi и мобильной сети
- [x] `XrayConfigBuilder`: конфигурация с выключенным access log
- [x] `RoutingPolicy`: два уровня разделения трафика
- [x] Интерфейс: одна кнопка и честный статус
- [x] `android-build.yml`: сборка APK с записью SHA-256
- [x] `libxray-build.yml`: сборка движка из исходников и дамп публичного API
- [x] `docs/architecture/android-client.md`, `ADR-020`, `ADR-021`
- [ ] Запустить `libXray Build` и получить публичную сигнатуру
- [ ] Написать привязку движка по фактической сигнатуре
- [ ] Business Owner: endpoint VLESS/REALITY для проверки
- [ ] Проверка на устройстве: ON, OFF, закрытие Activity, смена сети, DNS
