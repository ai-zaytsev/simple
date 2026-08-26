-- The Russian zone by name, not only by address.
--
-- The rule shipped as "Russian addresses go direct", which missed the case the
-- requirement names outright: a .ru site hosted abroad. 2ip.ru resolves to a
-- German address, so it went through the tunnel and showed a foreign one - the
-- Business Owner found it in the first ten minutes of looking.
--
-- Matching the zone by name catches it wherever the site is hosted, which is
-- what "even if they use foreign servers or a CDN" means.
--
-- Added here as well as through the operator workflow, because a database
-- rebuilt from migrations must come up with the same rules. A rule that exists
-- only because somebody once ran a workflow is a rule that disappears the day
-- the service is rebuilt.
insert into routing_rules (id, kind, value, note) values
    (gen_random_uuid(), 'direct_domain', 'domain:ru',       'Russian zone by name, whatever the site is hosted on'),
    (gen_random_uuid(), 'direct_domain', 'domain:su',       'Russian zone by name'),
    (gen_random_uuid(), 'direct_domain', 'domain:xn--p1ai', 'The Cyrillic zone, in the form the engine matches')
on conflict (kind, value) do update set enabled = true, note = excluded.note;
