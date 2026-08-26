-- Which traffic goes straight out and which goes through the tunnel.
--
-- Rows rather than code, because the whole requirement is that a wrong route
-- can be corrected without anybody installing anything. A rule that lives in
-- the application is a rule that takes a release, a review and a week to fix.
create table if not exists routing_rules (
    id         uuid primary key,

    -- What the value is, which decides both how it is matched and where in the
    -- order it sits. The order is the product rule and is fixed in code, not
    -- here: leaving it to data would let one careless row put "everything
    -- direct" above the explicit lists.
    kind       text not null check (kind in (
        'direct_domain',
        'direct_ip',
        'direct_app',
        'proxy_domain',
        'proxy_ip'
    )),

    value      text not null,

    -- Why this row exists, for whoever finds it in a year. A rule nobody can
    -- explain is a rule nobody dares remove.
    note       text not null default '',

    enabled    boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists routing_rules_unique on routing_rules (kind, value);
create index if not exists routing_rules_enabled on routing_rules (kind) where enabled;

-- The starting set, moved out of the application where it was hard-coded.
--
-- Deliberately short. A long guessed list creates false confidence: every
-- entry should come from somebody finding a route wrong, not from memory.
insert into routing_rules (id, kind, value, note) values
    (gen_random_uuid(), 'direct_app', 'ru.sberbankmobile',                'Bank: refuses to work from a foreign address'),
    (gen_random_uuid(), 'direct_app', 'ru.vtb24.mobilebanking.android',   'Bank'),
    (gen_random_uuid(), 'direct_app', 'ru.alfabank.mobile.android',       'Bank'),
    (gen_random_uuid(), 'direct_app', 'ru.tinkoff.sme',                   'Bank'),
    (gen_random_uuid(), 'direct_app', 'com.idamob.tinkoff.android',       'Bank'),
    (gen_random_uuid(), 'direct_app', 'ru.gosuslugi.app',                 'Government'),
    (gen_random_uuid(), 'direct_app', 'ru.nalog.fl',                      'Government'),
    (gen_random_uuid(), 'direct_domain', 'domain:gosuslugi.ru',           'Government'),
    (gen_random_uuid(), 'direct_domain', 'domain:nalog.ru',               'Government'),
    (gen_random_uuid(), 'direct_domain', 'domain:sberbank.ru',            'Bank'),
    (gen_random_uuid(), 'direct_domain', 'domain:vtb.ru',                 'Bank'),
    (gen_random_uuid(), 'direct_domain', 'domain:alfabank.ru',            'Bank'),
    (gen_random_uuid(), 'direct_domain', 'domain:tinkoff.ru',             'Bank'),
    (gen_random_uuid(), 'direct_domain', 'domain:mos.ru',                 'Government'),
    (gen_random_uuid(), 'direct_domain', 'domain:yandex.ru',              'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:mail.ru',                'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:vk.com',                 'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:ozon.ru',                'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:wildberries.ru',         'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:avito.ru',               'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:rt.ru',                  'Russian service'),
    (gen_random_uuid(), 'direct_domain', 'domain:2gis.ru',                'Russian service')
on conflict (kind, value) do nothing;
