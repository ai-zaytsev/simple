# libXray

Транспортный движок Android-клиента. Решение собирать из исходников, а не брать готовый артефакт — `ADR-020`.

## Что Собрано

Сборка workflow `libXray Build`, 2026-08-22.

| Параметр | Значение |
| --- | --- |
| Источник | `github.com/XTLS/libXray` |
| Коммит | `68119ddec6a16ea635838af0e9e11fd93316812c` |
| Xray-core | `v1.260327.1-0.20260728075948-5ca6f4b7d4dc` |
| Артефакт | `libXray.aar`, 95 485 903 байта |
| SHA-256 | `cc2d82586569adce1e5bf83df2674d0c1215641b612eb62142dd98568a3063d3` |

Версия Xray-core важнее версии обёртки: именно она обрабатывает пользовательский трафик. Поэтому она печатается в отчёт сборки отдельной строкой.

Размер AAR почти 96 МБ, потому что внутри нативные библиотеки под все ABI. В APK попадает только используемая архитектура, но это причина следить за итоговым размером сборки.

## Публичная Сигнатура

Получена дампом из собранного артефакта, а не из документации.

```java
public abstract class libXray.LibXray {
  public static final long LibXrayAPIVersion;
  public static void touch();
  public static native String invoke(String);
  public static native void registerDialerController(DialerController);
  public static native void registerListenerController(DialerController);
  public static native void registerProcessFinder(ProcessFinder, long);
  public static native void resetDNS();
  public static native void setDNS(DialerController, String) throws Exception;
}

public interface libXray.DialerController {
  boolean protectFd(long);
}

public interface libXray.ProcessFinder {
  long findProcessByConnection(String, String, long, String, long);
}
```

Управление идёт не через набор методов на каждую операцию, а через один `invoke`, принимающий и возвращающий строку. Типизированные классы вроде `LibXrayInvokeRequest` описывают конверт запроса и несут только версию API; содержательная часть команды передаётся внутри строки.

Следствие для привязки: контракт между приложением и движком — это формат команды, а не сигнатура метода. Ошибка в имени команды не будет поймана компилятором, поэтому набор используемых команд должен быть закрыт тестом.

## Главная Находка: `protectFd`

`DialerController.protectFd(long)` — движок отдаёт каждый создаваемый сокет, чтобы приложение вызвало `VpnService.protect()`. Это защита от петли на уровне движка: защищённый сокет идёт мимо TUN.

В клиенте уже есть защита на уровне ОС — приложение исключает само себя из туннеля, см. [../architecture/android-client.md](../architecture/android-client.md). Механизмы независимы и будут использованы оба:

- исключение приложения закрывает весь трафик процесса, включая служебный
- `protectFd` работает даже там, где исключение по пакету недоступно, и не зависит от того, что движок живёт внутри нашего процесса

Дублирование здесь оправдано: петля не деградирует соединение, а делает его неработоспособным незаметно, и стоимость второго механизма — один интерфейс.

## Что Ещё Не Установлено

Словарь команд `invoke`: имена операций запуска и остановки не видны в Java-сигнатуре, они заданы в Go-исходниках. Пока он не выяснен, привязка не пишется.

Это осознанная остановка. Написать привязку по догадке означает получить код, который собирается и не работает, а отличить такую поломку от проблем с сетью на устройстве заметно дороже, чем прочитать исходник.
