-- What has already been said about capacity, so that it is not said again
-- every five minutes.
--
-- An alert that repeats itself is an alert people filter, and a filtered
-- alert is the same as no alert with extra steps. So a state is announced when
-- it changes, and repeated only after long enough that somebody who missed it
-- deserves reminding.
--
-- Nothing here is about a user. It records that a sentence was sent, which
-- channel took it, and what the service believed at the time.
create table if not exists metrics.capacity_alerts (
    at      timestamptz not null default now(),
    state   text        not null check (state in ('NORMAL', 'WATCH', 'SCALE_REQUIRED', 'CRITICAL')),

    -- What the alert was about, so that a domain warning and a server warning
    -- do not silence each other.
    subject text        not null,

    -- Where it went. Several rows for one alert when several channels took it,
    -- because a channel that failed and one that worked are different facts.
    channel text        not null,
    ok      boolean     not null default true,
    detail  text        not null default '',

    primary key (at, subject, channel)
);

create index if not exists capacity_alerts_subject_idx
    on metrics.capacity_alerts (subject, at desc);

-- Which group of servers a node belongs to.
--
-- One group today. The column exists because the question "how much capacity
-- does this group have" has to be answerable before there are two, or the
-- answer arrives as a schema change during the week somebody needs it.
alter table nodes add column if not exists node_group text not null default 'default';
