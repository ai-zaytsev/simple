-- Access belongs to a device, not to a tier and not to an account.
--
-- Until now every client on a node presented the same VLESS identifier. That
-- made the four things this stage requires impossible at once: one device
-- could not be cut off without cutting off everybody, and a credential taken
-- from one phone was everybody's credential.

-- A device proves who it is with a secret, not by naming itself.
--
-- The identifier was a claim: anyone who learned one could send it and be
-- treated as that device. The token is issued when a mailbox is proved and
-- never travels again in the clear, so swapping an identifier now buys
-- nothing.
alter table devices add column if not exists token_hash bytea;
create unique index if not exists devices_token_idx on devices (token_hash)
    where token_hash is not null;

-- One credential per device, valid on every node.
--
-- Per device and not per device and node, because a reserve has to work the
-- moment the primary stops answering; a credential that exists on one node
-- only would turn every failover into a round trip to this service.
create table if not exists device_credentials (
    id              uuid primary key,
    device_id       uuid not null references devices (id) on delete cascade,

    -- What the node checks. Nothing else about the person reaches a node, so
    -- seizing one yields a list of these and no way to tie any of them to an
    -- address, an account, or each other.
    credential_uuid uuid not null unique,

    state           text not null default 'ACTIVE'
        check (state in ('ACTIVE', 'REVOKED')),

    created_at      timestamptz not null default now(),
    revoked_at      timestamptz,

    -- Bumped whenever the row changes, so a node can ask what changed since
    -- the number it last saw instead of re-reading the whole list.
    updated_seq     bigint not null default 0
);

-- One live credential per device. A device with two would leave one of them
-- unaccounted for when the device is cut off.
create unique index if not exists device_credentials_live_idx
    on device_credentials (device_id) where state = 'ACTIVE';

create index if not exists device_credentials_seq_idx on device_credentials (updated_seq);

-- Nodes authenticate too, and for the same reason devices do: the list of who
-- may connect is not public, and a node is the only thing that needs it.
alter table nodes add column if not exists token_hash bytea;
create unique index if not exists nodes_token_idx on nodes (token_hash)
    where token_hash is not null;
