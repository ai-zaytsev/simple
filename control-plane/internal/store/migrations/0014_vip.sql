-- The second tier.
--
-- A row rather than a column, a constraint or a migration to the accounts
-- table. That is what migration 0005 set up for exactly this moment: it wrote
-- down that "the column exists so that a second one is a row and an update,
-- not a migration under load", and this is the update.
--
-- The foreign key from accounts.tier means this insert is also what makes
-- 'VIP' a value an account is allowed to hold at all. Before it, setting one
-- would have been refused by the database rather than accepted and misread.
insert into tier_limits (tier, max_devices) values ('VIP', 1)
on conflict (tier) do nothing;

-- One device, the same as FREE, and that is deliberate rather than unfinished.
--
-- The stage that introduced VIP asked for the status and said nothing about
-- what it grants. Inventing a number here would be inventing product policy in
-- a migration, which is the worst place for it: invisible to whoever decides,
-- and expensive to argue with later.
--
-- Whatever the number should be, it is one statement and no deploy:
--
--     update tier_limits set max_devices = 3 where tier = 'VIP';
