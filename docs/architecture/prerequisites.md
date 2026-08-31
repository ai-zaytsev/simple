# Открытые внешние prerequisites

Здесь перечислены только ещё не выполненные внешние решения/доступы. Уже созданные серверы, домены, DNS, SaaS и их состояние находятся в [BO-инструкции](../business-owner-operations.md), поэтому прежний полный procurement checklist удалён как устаревший второй инвентарь.

## Сводка

| Что требуется | Когда блокирует | Почему нельзя заменить кодом |
| --- | --- | --- |
| Проверенная offline-копия Android release keystore и паролей | До первой официальной публикации | Потерянный signing identity нельзя восстановить из GitHub Secrets |
| Минимум ещё один несвязанный расходный домен | До следующего расширения/замены после использования последнего запаса | Регистрацию и оплату домена выполняет Business Owner |
| Резервный email provider | Не блокирует текущий MVP, снижает single-provider risk auth | Нужны аккаунт, sender verification и договор/лимиты |
| Production-магазин, юридический/фискальный контур и налоговая модель | До первой оплаты реальными деньгами | Test store не является правом принимать деньги |
| Решение о системном DNS resolver | До изменения текущего DNS profile | Это privacy/reliability trade-off, а не технический default |

Текущие блокеры выпуска ведутся в [release-blockers.md](../release-blockers.md), реализуемый долг — в [tech-debt.md](../tech-debt.md). Этот документ не авторизует покупку или создание ресурсов.

## Offline release signing

Business Owner хранит PKCS12 и пароли вне CI и проверяет, что копия читается. GitHub Secrets — рабочий канал, но значения нельзя получить обратно, поэтому они не являются backup. До проверки offline recovery первая официальная версия не публикуется.

## Запас расходных доменов

Новый домен должен:

- не продолжать очевидную числовую последовательность существующих;
- по возможности иметь другой TLD/регистратора;
- поддерживать API управления DNS;
- не включаться в клиентский bootstrap до фактического ввода;
- после покупки быть занесён в DNS inventory без публикации secrets.

Живое число пригодных доменов читается в BO-инструкции и Read The Panel.

## Резервный email provider

Adapter boundary уже существует. До переключения резерв обязан пройти:

1. sender/domain authentication;
2. plain-text magic link без click/open tracking;
3. доставку на Gmail и Yandex с неизменённой ссылкой;
4. server-only secrets;
5. provider status/readback без email в публичных логах.

Текущий Brevo не отключается ради проверки; переключение выполняется только после успешного параллельного smoke test.

## Production payments

До замены test credentials нужны:

- production contract/store;
- юридическое лицо/налоговый режим;
- обязательные данные чека и фискализация;
- утверждённые refund/chargeback и payment-data retention;
- пересчёт цен с фактическими налогами/комиссией;
- production smoke payment с каноническим provider status.

Переход test → production меняет только server-side provider configuration. Android, аккаунты, entitlement и VPN не получают provider secrets и не переделываются.

## DNS resolver

Открытый trade-off описан ADR-008. При решении проверяются доступность из целевых сетей, отсутствие утечки DNS мимо tunnel, политика логирования resolver и способность сменить его signed plan/config без APK update.
