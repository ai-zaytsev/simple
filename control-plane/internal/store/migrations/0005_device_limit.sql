-- How many devices one person may have connected at once.
--
-- A number per tier rather than a constant, because the number is the whole
-- difference between a free tier and a paid one, and it will change. A
-- constant would mean that raising it for paying customers requires a new
-- release of the application as well as of this service - which is exactly
-- the coupling the rest of this system exists to avoid.
create table if not exists tier_limits (
    tier        text primary key,
    max_devices int not null check (max_devices >= 1),
    updated_at  timestamptz not null default now()
);

insert into tier_limits (tier, max_devices) values ('FREE', 1)
on conflict (tier) do nothing;

-- Which tier an account is on. One value today; the column exists so that a
-- second one is a row and an update, not a migration under load.
alter table accounts add column if not exists tier text not null default 'FREE';

-- Every account is on a tier that has a limit. Without this an account could
-- carry a tier nobody had defined, and the lookup would find nothing - which
-- would have to be read as either "no limit" or "no devices", both wrong.
alter table accounts drop constraint if exists accounts_tier_known;
alter table accounts add constraint accounts_tier_known
    foreign key (tier) references tier_limits (tier);
