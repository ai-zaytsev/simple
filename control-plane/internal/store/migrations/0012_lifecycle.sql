-- Where a server or a domain is in its life, and what is currently wrong with
-- it. Two different questions, kept in two different places.
--
-- The distinction is the whole design. A lifecycle is declared: somebody, or
-- some automation, decides that this machine is being built, or is being taken
-- out of service, or has been isolated. A condition is observed: the
-- measurements say it is losing packets, or that devices cannot reach it.
--
-- Putting both in one column is the ordinary mistake and it fails in both
-- directions. An observer that writes "degraded" over "draining" loses the
-- operator's intent and the node starts being handed out again. An operator
-- who writes "ready" over "blocked" hides a live fault behind a stale opinion.
-- So intent is stored and condition is derived, every time, from what the
-- service has measured in the last few minutes.
--
-- One condition matters more than the rest and is the reason this stage
-- exists: a server that has broken and a server that works perfectly but
-- cannot be reached from Russia are not the same state, and no single vantage
-- point can tell them apart. Both look like silence from here. They are told
-- apart by measuring the same address from two places, which the service has
-- been doing since stage 12.

-- The vocabulary a node's life is described in.
--
-- `active` and `retired` are gone: they were three states doing the work of
-- ten, and "active" in particular answered two questions at once - whether the
-- machine is finished being built, and whether it should be handed out today.
alter table nodes drop constraint if exists nodes_state_check;

update nodes set state = 'serving' where state = 'active';
update nodes set state = 'removed' where state = 'retired';

alter table nodes add constraint nodes_state_check check (state in (
    -- Being built. Nothing about these is ready to be told to anybody.
    'creating',
    'configuring',
    'awaiting-certificate',
    'verifying',

    -- Finished and proven, but not yet carrying anybody. The difference
    -- between this and `serving` is deliberate: a node can be complete and
    -- deliberately held back, which is what makes a spare a spare.
    'ready',
    'serving',

    -- Leaving. `draining` still carries the people already on it and is given
    -- to nobody new; `quarantined` is a deliberate isolation, used when
    -- something is wrong that the measurements cannot see.
    'draining',
    'quarantined',

    -- Gone, or going. `removed` is kept rather than deleted so that a node
    -- alias is never reused and old measurements keep meaning something.
    'removing',
    'removed'
));

alter table nodes add column if not exists state_since timestamptz not null default now();
alter table nodes add column if not exists state_note text not null default '';

-- Domains as things in their own right.
--
-- Until now a domain existed only as a string inside a node's parameters or a
-- bootstrap row, which made "is this domain being retired" a question with
-- nowhere to write the answer. A consumable cover domain is a thing that gets
-- used up, and the whole point of the design is that we replace them; that is
-- a life, and it needs somewhere to be recorded.
create table if not exists domains (
    name text primary key,

    -- What the domain is for. A node's cover site and a way in to the Control
    -- Plane fail differently and are replaced differently, and treating them
    -- alike would mean retiring one the way you retire the other.
    purpose text not null check (purpose in ('node', 'bootstrap')),

    lifecycle text not null default 'ready' check (lifecycle in (
        'ready',
        'serving',
        'draining',
        'retired'
    )),

    since timestamptz not null default now(),
    note  text not null default '',

    created_at timestamptz not null default now()
);

create index if not exists domains_lifecycle_idx on domains (lifecycle);
