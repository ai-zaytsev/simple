-- Provider-neutral VIP refunds and the exact paid interval they reverse.

alter table payments add column if not exists payment_method text;
alter table payments add column if not exists provider_refundable boolean;
alter table payments add column if not exists entitlement_started_at timestamptz;
alter table payments add column if not exists entitlement_ends_at timestamptz;

-- Existing paid rows predate the interval snapshot. Prefer the account expiry
-- that was actually granted; if it has already been cleared by expiry, rebuild
-- the historical interval from the provider capture time.
update payments p
set entitlement_ends_at = coalesce(
        a.vip_expires_at,
        coalesce(p.paid_at, p.entitlement_applied_at) + make_interval(months => p.duration_months)
    ),
    entitlement_started_at = coalesce(
        a.vip_expires_at - make_interval(months => p.duration_months),
        p.paid_at,
        p.entitlement_applied_at
    )
from accounts a
where a.id = p.account_id
  and p.entitlement_applied_at is not null
  and (p.entitlement_started_at is null or p.entitlement_ends_at is null);

alter table payments drop constraint if exists payments_entitlement_interval_sane;
alter table payments add constraint payments_entitlement_interval_sane check (
    (entitlement_started_at is null and entitlement_ends_at is null)
    or
    (entitlement_started_at is not null and entitlement_ends_at > entitlement_started_at)
);

create table if not exists refunds (
    id                       uuid primary key,
    payment_id               uuid not null unique references payments (id),
    account_id               uuid not null references accounts (id),
    provider                 text not null,
    amount_minor             bigint not null check (amount_minor > 0),
    currency                 text not null check (currency = 'RUB'),
    mode                     text not null check (mode in ('full', 'pro_rata')),
    status                   text not null check (status in ('creating', 'pending', 'succeeded', 'canceled', 'failed')),
    cancellation_reason      text,
    calculated_at            timestamptz not null,
    created_at               timestamptz not null default now(),
    updated_at               timestamptz not null default now(),
    succeeded_at             timestamptz,
    entitlement_revoked_at   timestamptz,
    check (cancellation_reason is null or cancellation_reason <> '')
);

create table if not exists refund_attempts (
    id                       uuid primary key,
    refund_id                uuid not null references refunds (id),
    provider                 text not null,
    attempt_no               int not null check (attempt_no > 0),
    idempotency_key          uuid not null unique,
    provider_refund_id       text,
    status                   text not null check (status in ('creating', 'pending', 'succeeded', 'canceled', 'failed')),
    cancellation_reason      text,
    created_at               timestamptz not null default now(),
    updated_at               timestamptz not null default now(),
    unique (refund_id, attempt_no),
    unique (provider, provider_refund_id),
    check (provider_refund_id is null or provider_refund_id <> ''),
    check (cancellation_reason is null or cancellation_reason <> '')
);

create unique index if not exists refund_attempts_provider_id
    on refund_attempts (provider, provider_refund_id)
    where provider_refund_id is not null;

create unique index if not exists refund_attempts_one_live
    on refund_attempts (refund_id)
    where status in ('creating', 'pending');

create unique index if not exists refund_attempts_one_success
    on refund_attempts (refund_id)
    where status = 'succeeded';

create index if not exists refunds_account_history
    on refunds (account_id, created_at desc);

