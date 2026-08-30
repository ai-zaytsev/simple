-- One version policy for every distribution channel. The minimum is copied
-- from the original root setting so deployment cannot silently relax a stop
-- that an operator already made.
insert into service_state (key, value, changed_by)
select
    'app_updates',
    jsonb_build_object(
        'latest_version_code', 1,
        'latest_version_name', '0.1.0',
        'min_supported_version_code',
            coalesce((select (value #>> '{}')::int
                      from service_state
                      where key = 'min_supported_app_version'), 1),
        'channels', '{}'::jsonb
    ),
    'migration'
on conflict (key) do nothing;
