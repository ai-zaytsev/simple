-- Sign-in by email link.
--
-- The rule that shapes this schema is that the application must never reveal
-- whether an address is registered. So an attempt is recorded the same way for
-- an address that exists and one that does not, and the account is created when
-- the link is followed rather than when it is requested. Someone typing a
-- stranger's address learns nothing and creates nothing.

alter table accounts add column if not exists email text;
alter table accounts add column if not exists state text not null default 'ACTIVE'
    check (state in ('PENDING', 'ACTIVE', 'SUSPENDED', 'DELETION_REQUESTED'));

-- Addresses are compared in one normalised form, so that a person who signs up
-- as one spelling and returns as another reaches the same account rather than a
-- second one nobody can find.
create unique index if not exists accounts_email_idx on accounts (lower(email))
    where email is not null;

create table if not exists login_attempts (
    id            uuid primary key,
    email         text not null,
    device_id     uuid not null,

    -- Only the hash is kept. The link in the mailbox is the secret; a database
    -- copy of it would let anyone reading the database sign in as anyone, which
    -- is a worse loss than the database itself.
    token_hash    bytea not null unique,

    created_at    timestamptz not null default now(),
    expires_at    timestamptz not null,
    confirmed_at  timestamptz,

    -- Set when the link is followed. A second visit finds it already set and is
    -- refused, which is what makes the link one-time rather than
    -- one-time-in-practice.
    consumed_at   timestamptz,

    account_id    uuid references accounts (id) on delete set null
);

create index if not exists login_attempts_email_idx on login_attempts (lower(email), created_at desc);
create index if not exists login_attempts_device_idx on login_attempts (device_id, created_at desc);

-- Bound to the account only after the address is proven. Until then a device
-- has no account, which is what makes every fresh install confirm separately.
alter table devices alter column account_id drop not null;
