-- Чеки НПД: что должно существовать в «Мой налог» для каждого платежа.
--
-- Устройство таблиц выведено из одного требования: «не допускать двух чеков на
-- один и тот же платёж» и «повторной отмены одного чека». Оба запрета здесь
-- держит база, а не аккуратность вызывающего кода: аккуратность переживает
-- ровно до первой гонки между веб-хуком и разбором очереди.

create table if not exists npd_receipts (
    id              uuid primary key default gen_random_uuid(),
    payment_id      uuid not null references payments(id) on delete restrict,
    receipt_uuid    text,
    amount_minor    bigint not null check (amount_minor > 0),
    -- 'creating' is the window between asking ФНС for a receipt and learning
    -- its identifier. A crash there must not become a second receipt, so the
    -- row exists before the call and blocks another attempt until a person
    -- looks. A duplicate receipt is worse than a stopped queue.
    state           text not null check (state in ('creating', 'active', 'cancelled')),
    print_url       text,
    created_at      timestamptz not null default now(),
    cancelled_at    timestamptz,

    -- Отменённый чек обязан помнить, когда он отменён, а действующий — не
    -- притворяться отменённым.
    constraint npd_receipts_cancelled_has_time check (
        (state = 'cancelled') = (cancelled_at is not null)
    )
);

-- Два чека на один платёж не могут существовать одновременно, и попытка,
-- исход которой неизвестен, считается чеком. Это тот самый
-- запрет из задания, и он стоит здесь, а не в Go.
create unique index if not exists npd_receipts_one_active
    on npd_receipts (payment_id) where state in ('creating', 'active');

create index if not exists npd_receipts_payment on npd_receipts (payment_id);

-- Устойчивая очередь.
--
-- Одна разновидность операции, а не «создать» и «отменить» отдельно: обработчик
-- считает, каким чек ДОЛЖЕН быть (сумма платежа минус подтверждённые возвраты),
-- и приводит НПД к этому виду. Поэтому повтор после падения посередине —
-- обычный повтор, а не особый случай: посчитает заново и доделает.
create table if not exists npd_operations (
    id              uuid primary key default gen_random_uuid(),
    payment_id      uuid not null references payments(id) on delete restrict,
    state           text not null default 'pending'
                        check (state in ('pending', 'done', 'failed')),
    attempts        integer not null default 0,
    last_error      text,

    -- Когда про эту неудачу написали Business Owner. Письмо на каждый
    -- неудавшийся чек — одно, а не на каждую попытку: письмо на попытку это
    -- письмо, которое перестают читать.
    alerted_at      timestamptz,

    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now()
);

-- Одна незакрытая операция на платёж. Возврат по платежу, который ещё ждёт
-- своего чека, не создаёт вторую очередь - он попадёт в ту же и будет учтён,
-- потому что сумма считается в момент обработки, а не в момент постановки.
create unique index if not exists npd_operations_one_open
    on npd_operations (payment_id) where state = 'pending';

create index if not exists npd_operations_pending
    on npd_operations (created_at) where state = 'pending';

-- Сессия к lknpd. Одна строка.
--
-- В базе, а не в файле: файл на хосте не попадает в резервную копию и исчезает
-- при переносе. База шифруется в копии целиком. Ни одно поле отсюда не имеет
-- права попасть в лог.
create table if not exists npd_session (
    id              boolean primary key default true check (id),
    inn             text,
    device_id       text not null,
    access_token    text,
    refresh_token   text,
    expires_at      timestamptz,
    updated_at      timestamptz not null default now()
);

-- Доступность ФНС, как её увидела последняя полноценная проверка. Одна строка.
create table if not exists npd_availability (
    id              boolean primary key default true check (id),
    ok              boolean not null default false,
    checked_at      timestamptz,
    detail          text
);

-- Пока проверка не проходила ни разу, продажи закрыты. Это осознанно: «мы ещё
-- не знаем» и «всё хорошо» — разные состояния, и продавать без чека нельзя ни
-- в одном из них.
insert into npd_availability (id, ok, detail)
values (true, false, 'проверка ещё не выполнялась')
on conflict (id) do nothing;

-- Действующий чек обязан иметь идентификатор; строка в состоянии 'creating' —
-- ровно та, у которой его ещё нет.
alter table npd_receipts drop constraint if exists npd_receipts_active_has_uuid;
alter table npd_receipts add constraint npd_receipts_active_has_uuid check (
    (state = 'creating') = (receipt_uuid is null)
);
