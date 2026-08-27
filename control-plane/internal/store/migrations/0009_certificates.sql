-- What has been issued, and when.
--
-- Kept for one reason: Let's Encrypt allows five certificates a week for the
-- same set of names, and running into that limit means a node cannot renew for
-- days. A limit reached by accident is indistinguishable from a limit reached
-- by an attack, and neither can be argued with once it is hit - so it is
-- counted here rather than hoped about.
--
-- Nothing secret is stored. A certificate is meant to be shown to everybody,
-- and the private key never leaves the node that made it.
create table if not exists certificate_issues (
    id         uuid primary key,

    -- The name that was certified. One row per issuance, not per node: the
    -- authority counts by name, so that is what has to be counted here.
    name       text not null,

    -- Which node asked, so that a node asking repeatedly can be told apart
    -- from a domain being renewed by several nodes in turn.
    node_alias text not null,

    issued_at  timestamptz not null default now(),
    expires_at timestamptz,

    -- Set when an issuance was refused rather than granted, with the reason.
    -- Refusals are as worth keeping as successes: a node that keeps being told
    -- no is a node nobody would otherwise notice.
    refused    boolean not null default false,
    reason     text not null default ''
);

create index if not exists certificate_issues_name_idx
    on certificate_issues (name, issued_at desc);
