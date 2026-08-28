-- Devices that are not our application.
--
-- A router, a television, a computer, or any client that speaks the same
-- protocol. They hold a link and nothing else: they never ask for a plan,
-- never report, and know nothing about this service beyond one address.
--
-- A row in devices rather than a table of their own, because everything that
-- has to be shared already is: access is a row in device_credentials, taking
-- it away is the same statement, and nodes learn who may connect through the
-- same query. A second table would mean a second way to revoke, and a second
-- way to revoke is the one somebody eventually forgets to use.
alter table devices add column if not exists kind text not null default 'app';

alter table devices drop constraint if exists devices_kind_known;
alter table devices add constraint devices_kind_known
    check (kind in ('app', 'external'));

-- What the person calls it. "Телевизор", "роутер в гостиной". Shown back to
-- them so that revoking the right one does not require reading identifiers.
alter table devices add column if not exists label text not null default '';

-- How many external devices a tier may have.
--
-- Separate from max_devices, and that separation is the whole point: the two
-- limits count different things and must not evict each other. With one shared
-- number, adding a television on a tier that allows one device would cut off
-- the phone that added it.
alter table tier_limits add column if not exists max_external int not null default 0;

-- Zero for FREE, which follows directly from the stage: the ability is given
-- to VIP.
update tier_limits set max_external = 0 where tier = 'FREE';

-- Five for VIP, and that number is a placeholder of exactly the same kind as
-- the device limit beside it. How many televisions and routers a paying
-- customer may connect is a decision for whoever sells the product, not a
-- consequence of building it. One statement changes it, with no deploy:
--
--     update tier_limits set max_external = 10 where tier = 'VIP';
--
-- It is not unlimited, and that is deliberate: the stage forbids one shared
-- VIP key, and an account able to mint access without end is the same thing
-- arrived at by a different route.
update tier_limits set max_external = 5 where tier = 'VIP';
