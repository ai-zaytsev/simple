-- Provider-neutral one-time purchases and expiring paid VIP.
--
-- The provider is deliberately a value in a row, not a table shape. Adding a
-- second acquirer must add an adapter and must not add a second set of account
-- or entitlement columns.

alter table accounts add column if not exists vip_expires_at timestamptz;
alter table accounts drop constraint if exists accounts_vip_expiry_sane;
alter table accounts add constraint accounts_vip_expiry_sane
    check (vip_expires_at is null or tier = 'VIP');

create table if not exists payment_products (
    id              text primary key,
    title           text not null,
    amount_minor    bigint not null check (amount_minor > 0),
    currency        text not null check (currency = 'RUB'),
    duration_months int not null check (duration_months > 0),
    active          boolean not null default true,
    created_at      timestamptz not null default now(),
    updated_at      timestamptz not null default now()
);

insert into payment_products (id, title, amount_minor, currency, duration_months) values
    ('vip_1_month', 'VIP на 1 месяц', 39900, 'RUB', 1),
    ('vip_3_months', 'VIP на 3 месяца', 109000, 'RUB', 3),
    ('vip_12_months', 'VIP на 12 месяцев', 349000, 'RUB', 12)
on conflict (id) do nothing;

create table if not exists payments (
    id                      uuid primary key,
    account_id              uuid not null references accounts (id),
    product_id              text not null references payment_products (id),
    provider                text not null,
    provider_payment_id     text,
    idempotency_key         uuid not null unique,

    -- Snapshot the commercial promise. A later price edit must not change
    -- what an already-created payment buys or what a webhook is checked
    -- against.
    amount_minor            bigint not null check (amount_minor > 0),
    currency                text not null check (currency = 'RUB'),
    duration_months         int not null check (duration_months > 0),

    status                  text not null
                            check (status in ('creating', 'pending', 'succeeded',
                                              'canceled', 'failed')),
    checkout                text,
    provider_test           boolean,
    created_at              timestamptz not null default now(),
    updated_at              timestamptz not null default now(),
    paid_at                 timestamptz,
    entitlement_applied_at  timestamptz,

    unique (provider, provider_payment_id),
    check (provider_payment_id is null or provider_payment_id <> ''),
    check (checkout is null or checkout <> '')
);

-- A retry returns the same open purchase instead of creating another payable
-- page. This is per account, not per product: three simultaneous checkouts for
-- three terms would let all three be paid after the first had already made the
-- account VIP.
create unique index if not exists payments_one_open_per_account
    on payments (account_id)
    where status in ('creating', 'pending');

create index if not exists payments_account_history
    on payments (account_id, created_at desc);

create unique index if not exists payments_provider_id
    on payments (provider, provider_payment_id)
    where provider_payment_id is not null;
