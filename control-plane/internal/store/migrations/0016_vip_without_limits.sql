-- VIP has no limit on devices, of either kind.
--
-- Absence of a limit is written as absence of a value, not as zero. Zero is
-- already taken and means the opposite: FREE has zero external devices, which
-- is none at all. One column cannot carry "none" and "no limit" as the same
-- number, and a reader who confused them would either lock a paying customer
-- out or hand a free one everything.
alter table tier_limits alter column max_devices drop not null;
alter table tier_limits alter column max_external drop not null;

-- The check allowed only positive numbers, which made "no limit" impossible to
-- write down. Replaced rather than dropped: a negative device count is still
-- nonsense and should still be refused.
alter table tier_limits drop constraint if exists tier_limits_max_devices_check;
alter table tier_limits add constraint tier_limits_max_devices_sane
    check (max_devices is null or max_devices >= 0);

alter table tier_limits drop constraint if exists tier_limits_max_external_sane;
alter table tier_limits add constraint tier_limits_max_external_sane
    check (max_external is null or max_external >= 0);

update tier_limits set max_devices = null, max_external = null where tier = 'VIP';
