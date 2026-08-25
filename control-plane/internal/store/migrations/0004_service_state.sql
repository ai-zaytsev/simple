-- What the operator can change about the service without deploying anything.
--
-- The kill switch existed in the document from the first version and was
-- always false, because nothing could set it. A switch nobody can flip is not
-- a switch; it is a field. This is what makes it one.
--
-- One table of named values rather than a column per setting: adding a setting
-- must not mean a migration, a deploy and a client that has to understand a
-- new shape all at once.
create table if not exists service_state (
    key        text primary key,
    value      jsonb not null,
    updated_at timestamptz not null default now(),

    -- Who last changed it, in the loosest sense: a workflow run, a person, a
    -- script. Free text because the point is to have something to ask about
    -- later, not to build an authorisation model on.
    changed_by text
);

-- Defaults are inserted rather than left absent, so that the state of the
-- service is always readable as a row and never as the absence of one.
insert into service_state (key, value, changed_by) values
    ('kill_switch', '{"enabled": false, "message_key": ""}'::jsonb, 'migration'),
    ('min_supported_app_version', '1'::jsonb, 'migration')
on conflict (key) do nothing;
