-- VIP has no device limits at all.
--
-- Said plainly by the Business Owner, alongside the speed limit: FREE is held
-- to ten megabits and one device, VIP to neither. Migrations 0014 and 0015
-- left placeholders - one device, five external - and both said in their own
-- comments that the number was a decision for whoever sells the product
-- rather than a consequence of building it. This is that decision arriving.
--
-- Null is no limit, which is not a new idea invented here: 0017 already
-- established it for speed, for the reason that zero means "none at all" and
-- one column must not carry both meanings. Two columns expressing "unlimited"
-- two different ways would be the more expensive choice.

-- The check from 0005 required at least one device and would refuse null.
-- Postgres named it itself, which is why it is dropped by that name.
alter table tier_limits drop constraint if exists tier_limits_max_devices_check;
alter table tier_limits alter column max_devices drop not null;
alter table tier_limits drop constraint if exists tier_limits_devices_sane;
alter table tier_limits add constraint tier_limits_devices_sane
    check (max_devices is null or max_devices >= 1);

alter table tier_limits alter column max_external drop not null;
alter table tier_limits drop constraint if exists tier_limits_external_sane;
alter table tier_limits add constraint tier_limits_external_sane
    check (max_external is null or max_external >= 0);

-- FREE is untouched on purpose. One application installation, no external
-- devices, ten megabits - that is the whole of what separates the tiers now,
-- and the separation is the product.
update tier_limits set max_devices = null, max_external = null where tier = 'VIP';
