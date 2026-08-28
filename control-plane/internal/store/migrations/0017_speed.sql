-- How fast a tier may go, in megabits per second.
--
-- Null is no limit, the same way it is for devices: zero would mean "no
-- traffic at all", which is a real and different thing, and one column must
-- not carry both.
alter table tier_limits add column if not exists speed_mbit int;

alter table tier_limits drop constraint if exists tier_limits_speed_sane;
alter table tier_limits add constraint tier_limits_speed_sane
    check (speed_mbit is null or speed_mbit > 0);

update tier_limits set speed_mbit = 10   where tier = 'FREE';
update tier_limits set speed_mbit = null where tier = 'VIP';
