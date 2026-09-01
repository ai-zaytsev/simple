# Plan: Закрытие живой приемки ЮKassa

- Feature ID: `164-yookassa-live-closeout`
- Feature Branch: `feature/164-yookassa-live-closeout`

1. Обновить durable verifier wording и unit expectation.
2. Записать точные live outcomes и provider limitation в BO/integration/debt.
3. Закрыть feature-memory tasks 157, 161, 163 и сам closeout.
4. Выполнить локальные проверки и PR loop.
5. После human merge ещё раз прочитать панель.

Риск — объявить непроверенное проверенным. Поэтому terminal cancel отделяется от
card-attempt failure, webhook доказывается DB status до read/reconcile, а refund
insufficient balance остаётся явно непроверяемым без provider trigger.
