-- Which kind of user a kind of traffic came from, without saying which user.
--
-- Stage 12 kept traffic by kind and volume by person strictly apart, because
-- the pair would be a profile: "this person is ninety percent video" is the
-- thing this service must never be able to say. That separation stands.
--
-- What stage 13 asks for is different and answerable: how the heaviest users
-- differ from everyone else. That is a comparison between two groups, not a
-- fact about anybody, so the dimension added here has exactly two values and
-- the database refuses any third. A column that can only ever hold `ordinary`
-- or `heavy` cannot grow into a user key, however anybody later decides to
-- fill it.
--
-- The split is made on the node, at the moment of routing, by sending the
-- traffic of heavier accounts through a parallel set of outbounds. Nothing
-- about who they are travels with it: what arrives here is two sets of byte
-- counters, and the node forgets which was which as soon as it has read them.
alter table metrics.traffic_classes
    add column if not exists cohort text not null default 'ordinary';

do $$
begin
    if not exists (
        select 1 from pg_constraint where conname = 'traffic_classes_cohort_check'
    ) then
        alter table metrics.traffic_classes
            add constraint traffic_classes_cohort_check
            check (cohort in ('ordinary', 'heavy'));
    end if;
end $$;

-- The primary key gains the cohort, so the two groups accumulate separately
-- for the same minute and class rather than overwriting one another.
alter table metrics.traffic_classes drop constraint if exists traffic_classes_pkey;
alter table metrics.traffic_classes
    add constraint traffic_classes_pkey primary key (node_alias, at, class, cohort);

alter table metrics.traffic_class_days
    add column if not exists cohort text not null default 'ordinary';

do $$
begin
    if not exists (
        select 1 from pg_constraint where conname = 'traffic_class_days_cohort_check'
    ) then
        alter table metrics.traffic_class_days
            add constraint traffic_class_days_cohort_check
            check (cohort in ('ordinary', 'heavy'));
    end if;
end $$;

alter table metrics.traffic_class_days drop constraint if exists traffic_class_days_pkey;
alter table metrics.traffic_class_days
    add constraint traffic_class_days_pkey primary key (day, class, cohort);

-- How many people were in each group when the traffic was measured.
--
-- Kept because a share is meaningless without it, and because it is what the
-- panel checks before showing anything: a "heavy cohort" of two people is not
-- a group, it is two people, and describing what they do is describing them.
create table if not exists metrics.cohort_sizes (
    at       timestamptz not null,
    cohort   text        not null check (cohort in ('ordinary', 'heavy')),
    accounts int         not null default 0,
    primary key (at, cohort)
);
