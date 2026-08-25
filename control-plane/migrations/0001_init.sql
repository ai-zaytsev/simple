-- Control Plane schema.
--
-- Two things here are load bearing and easy to lose in a later migration.
--
-- The sequence counter lives in a table, not in the process. Invariant I-2 in
-- docs/architecture/evolution.md: with a counter in memory, a second instance
-- of the Control Plane would issue documents carrying equal or decreasing
-- numbers, and every client would reject valid configuration as an attempted
-- rollback. The counter is therefore transactional from the first day, when
-- there is exactly one instance and it does not yet matter.
--
-- Every issued document is kept. That is what makes "go back to the last
-- working configuration" possible at all: rolling back is re-issuing an older
-- payload under a new, higher number, because clients refuse anything that
-- moves backwards.

create table if not exists accounts (
    id          uuid primary key,
    tier        text not null default 'FREE',
    created_at  timestamptz not null default now()
);

create table if not exists devices (
    id            uuid primary key,
    account_id    uuid not null references accounts (id) on delete cascade,
    created_at    timestamptz not null default now(),
    last_seen_at  timestamptz
);

create index if not exists devices_account_idx on devices (account_id);

-- A node is described by what a client needs to reach it and nothing else.
-- No region, no provider, no capacity: the plan must not let a client infer
-- the shape of the fleet, so the database does not carry what it must not
-- hand out.
create table if not exists nodes (
    id              uuid primary key,
    alias           text not null unique,
    host            text not null,
    port            integer not null,
    transport_kind  text not null,
    params          jsonb not null,
    state           text not null default 'active'
                    check (state in ('active', 'draining', 'retired')),
    created_at      timestamptz not null default now()
);

create index if not exists nodes_state_idx on nodes (state);

create table if not exists doc_seq (
    scope  text primary key,
    value  bigint not null default 0
);

create table if not exists documents (
    id         bigserial primary key,
    kind       text not null,
    scope      text not null,
    seq        bigint not null,
    payload    jsonb not null,
    issued_at  timestamptz not null default now(),
    unique (kind, scope, seq)
);

create index if not exists documents_lookup_idx on documents (kind, scope, seq desc);

-- Returns the next number for a scope, atomically.
--
-- Written as an upsert rather than a read followed by a write, because two
-- concurrent requests reading the same value and both adding one is exactly
-- the collision this exists to prevent.
create or replace function next_seq(p_scope text) returns bigint as $$
declare
    v bigint;
begin
    insert into doc_seq (scope, value)
    values (p_scope, 1)
    on conflict (scope) do update set value = doc_seq.value + 1
    returning value into v;
    return v;
end;
$$ language plpgsql;
