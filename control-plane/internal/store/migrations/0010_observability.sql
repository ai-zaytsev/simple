-- What the service is doing, in a shape that cannot become a browsing history.
--
-- The guarantee here is structural rather than procedural. There is no column
-- anywhere below for a destination, a domain, a DNS query, a URL, an SNI or a
-- resource address, so no retention period, no leak and no subpoena turns
-- these tables into a record of where anybody went. Nothing has to be scrubbed
-- on the way in, because there is nowhere for it to go.
--
-- Two separations do the real work, and both are worth stating because both
-- are easy to undo by accident later:
--
--   1. Traffic by kind has no user column. Traffic by user has no kind column.
--      Either one alone is harmless; the pair would be a profile. They are not
--      merely unjoined - the numbers arrive from Xray as two independent sets
--      of counters, and Xray offers no way to ask for the cross product.
--
--   2. Nothing here carries an account. The key is the epoch pseudonym from
--      internal/analytics, which the Control Plane can compute and nobody can
--      reverse. When an epoch key is dropped, the link to a person is gone
--      from data that has already been written.
--
-- Kept in its own schema, with its own tables and no foreign key to anything
-- in public. That is not tidiness: it is what makes moving this to its own
-- database later a change of connection string rather than a rewrite. Nothing
-- in the service may join across the boundary, and if nothing joins, the two
-- halves can live on different machines the day the traffic asks for it.
create schema if not exists metrics;

-- How a node is doing. One row per node per reporting window.
create table if not exists metrics.node_samples (
    at                    timestamptz not null,
    node_alias            text        not null,
    window_s              int         not null,

    users_configured      int,
    sessions_online       int,

    cpu_percent           real,
    load1                 real,
    memory_percent        real,
    established           int,

    -- Bytes carried in the window, both directions, for the whole node.
    uplink_bytes          bigint      not null default 0,
    downlink_bytes        bigint      not null default 0,

    -- The node's own network health, measured against a fixed anchor of our
    -- choosing. Not against anywhere a user went.
    upstream_latency_ms   real,
    upstream_loss_percent real,

    goroutines            int,
    heap_bytes            bigint,
    xray_uptime_s         bigint,

    primary key (node_alias, at)
);

-- Traffic by kind. Node-wide totals, deliberately with no user column.
create table if not exists metrics.traffic_classes (
    at             timestamptz not null,
    node_alias     text        not null,
    class          text        not null,
    uplink_bytes   bigint      not null default 0,
    downlink_bytes bigint      not null default 0,
    primary key (node_alias, at, class)
);

-- Volume per user per period, deliberately with no kind column.
--
-- The period is a month, and the pseudonym is derived from the month's start,
-- so one person's month is one row however often the epoch key rotates.
create table if not exists metrics.account_usage (
    period       date   not null,
    analytics_id text   not null,
    bytes        bigint not null default 0,
    primary key (period, analytics_id)
);

-- Who was using the service in a given hour, as a pseudonym.
--
-- This is how "how many users are active" is answered without keeping a list
-- of people. Counting distinct rows gives the number; the rows themselves lead
-- nowhere once the epoch key is gone.
create table if not exists metrics.active_hours (
    hour         timestamptz not null,
    analytics_id text        not null,
    primary key (hour, analytics_id)
);

-- What devices report about reaching us: attempts, outcomes, how long
-- sessions lasted. About the connection to our service, never about what
-- travelled through it.
--
-- Already summed by the time it is stored. There is no row per attempt and no
-- device column, so this cannot be read backwards into one person's pattern of
-- use.
create table if not exists metrics.connect_reports (
    at              timestamptz not null,
    node_alias      text        not null default '',
    entry_kind      text        not null default '',
    attempts        int         not null default 0,
    successes       int         not null default 0,
    reconnects      int         not null default 0,
    session_seconds bigint      not null default 0,
    latency_ms_sum  bigint      not null default 0,
    latency_samples int         not null default 0
);

create index if not exists connect_reports_at_idx on metrics.connect_reports (at desc);

-- Our own checks of our own addresses.
--
-- `target` is a domain, and it is the one domain column in this schema. It
-- holds names we serve and nothing else - the service refuses to write a name
-- that is not one of ours. This is the sensor the stage asks for: blocking is
-- found by testing our own way in, not by reading where users failed to go.
create table if not exists metrics.endpoint_probes (
    at         timestamptz not null,
    target     text        not null,
    vantage    text        not null,
    ok         boolean     not null,
    latency_ms int,
    -- A short outcome from a fixed vocabulary, never a server's own words.
    -- An error message repeated verbatim is how a destination ends up in a
    -- database that has no column for one.
    detail     text        not null default ''
);

create index if not exists endpoint_probes_at_idx on metrics.endpoint_probes (target, at desc);

-- Daily summaries, so that answering "how was last spring" never means reading
-- a year of minute rows. The minute rows are kept for a few weeks and dropped;
-- these stay.
create table if not exists metrics.node_days (
    day               date   not null,
    node_alias        text   not null,
    uplink_bytes      bigint not null default 0,
    downlink_bytes    bigint not null default 0,
    peak_sessions     int    not null default 0,
    avg_cpu_percent   real,
    max_cpu_percent   real,
    avg_latency_ms    real,
    max_loss_percent  real,
    samples           int    not null default 0,
    primary key (node_alias, day)
);

create table if not exists metrics.traffic_class_days (
    day            date   not null,
    class          text   not null,
    uplink_bytes   bigint not null default 0,
    downlink_bytes bigint not null default 0,
    primary key (day, class)
);
