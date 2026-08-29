-- Whether VIP may be bought, and how long a new account waits first.
--
-- Two settings in the table migration 0004 made for exactly this: named values
-- rather than a column each, so that adding one is an insert and not a
-- migration, a deploy and a client that has to learn a new shape all at once.
--
-- Both are set from the server on purpose. The stage says the wait must change
-- without an APK update, and the reason generalises: a number that can only be
-- changed by releasing an application is a number nobody changes. The switch
-- is there for the day something is wrong with taking money, which is a day
-- that arrives without warning and must not need a release to survive.
insert into service_state (key, value, changed_by) values
    -- Off to begin with, and that is not timidity. Nothing can take a payment
    -- yet - no provider, no checkout, nothing - so a button that offered to
    -- sell would be lying. Turning this on is the one action that makes the
    -- offer real, and it should be somebody's deliberate decision rather than
    -- the state a migration happened to leave behind.
    ('purchases', '{"open": false, "free_days": 7}'::jsonb, 'migration')
on conflict (key) do nothing;
