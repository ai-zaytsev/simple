-- Every way a client can reach this service.
--
-- Rows rather than an environment variable, because the point of the whole
-- mechanism is that a blocked entry can be replaced without deploying, without
-- building and without anybody installing anything. A list that lives in the
-- configuration of the process is a list that needs a deploy to change, and a
-- deploy needs this service to be reachable - which is exactly what is in
-- question when it matters.
create table if not exists bootstrap_entries (
    id          uuid primary key,

    -- How to use the host. See document.BootstrapEntry for what each means.
    kind        text not null check (kind in ('https-direct', 'https-ip', 'https-edge')),

    host        text not null,
    port        int  not null default 443,

    -- Carried in the handshake when the host is an address, and the path an
    -- edge forwards from. Empty for a plain name.
    server_name text not null default '',
    path_prefix text not null default '',

    -- Orders the attempts. Equal weights are fine: the client shuffles within
    -- them, which is deliberate - one order for every client is a signature.
    weight      int not null default 100 check (weight > 0),

    note        text not null default '',
    enabled     boolean not null default true,
    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now()
);

create unique index if not exists bootstrap_entries_unique
    on bootstrap_entries (kind, host, path_prefix);

-- The starting set: three ways in that fail for different reasons.
--
-- The name is the ordinary path and the first to go when a domain is blocked.
-- The address needs no resolver at all, so a poisoned or blocked one does not
-- touch it; it falls only with the machine's address. The edge is a different
-- machine, in a different country, at a different provider, on a domain from a
-- different registrar - so it shares no failure with either.
insert into bootstrap_entries (id, kind, host, port, server_name, path_prefix, weight, note) values
    (gen_random_uuid(), 'https-direct', 'simple-syncbridge.download', 443, '', '', 100,
     'The ordinary path'),
    (gen_random_uuid(), 'https-ip', '185.9.26.52', 443, 'simple-syncbridge.download', '', 80,
     'Same machine without a resolver: survives DNS blocking')
on conflict (kind, host, path_prefix) do nothing;
