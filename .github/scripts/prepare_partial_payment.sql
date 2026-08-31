begin;

create temporary table prepared_payment (
    id uuid primary key,
    account_id uuid not null,
    entitlement_ends_at timestamptz not null
) on commit drop;

select set_config('simple.acceptance_prefix', :'prefix', true) as configured \gset

do $$
declare
    target_account_id uuid;
    target_tier text;
    target_payment_id uuid;
    target_duration int;
    new_start timestamptz := statement_timestamp() - interval '8 days';
    new_end timestamptz;
begin
    -- STRICT is the ambiguity guard: zero or two prefix matches abort the
    -- whole transaction before a payment row is selected.
    select a.id, a.tier
    into strict target_account_id, target_tier
    from accounts a
    where a.id::text like current_setting('simple.acceptance_prefix') || '%'
    for update;

    if target_tier <> 'VIP' then
        raise exception 'the test account is not VIP';
    end if;

    select p.id, p.duration_months
    into strict target_payment_id, target_duration
    from payments p
    where p.account_id = target_account_id
      and p.status = 'succeeded'
      and p.provider = 'yookassa'
      and p.provider_test is true
      and p.provider_refundable is true
      and p.payment_method = 'bank_card'
      and p.entitlement_applied_at is not null
      and p.entitlement_started_at is not null
      and p.entitlement_ends_at is not null
      and not exists (select 1 from refunds r where r.payment_id = p.id)
    order by p.created_at desc
    limit 1
    for update;

    new_end := new_start + make_interval(months => target_duration);
    if new_end <= statement_timestamp() then
        raise exception 'the prepared paid interval would already be over';
    end if;

    update payments
    set paid_at = new_start,
        entitlement_applied_at = new_start,
        entitlement_started_at = new_start,
        entitlement_ends_at = new_end,
        updated_at = statement_timestamp()
    where id = target_payment_id;

    update accounts
    set vip_expires_at = new_end
    where id = target_account_id and tier = 'VIP';
    if not found then
        raise exception 'the test account changed while it was prepared';
    end if;

    insert into prepared_payment (id, account_id, entitlement_ends_at)
    values (target_payment_id, target_account_id, new_end);
end $$;

select jsonb_build_object(
    'prepared', count(*),
    'account_updated', count(*),
    'payment_prefix', min(left(id::text, 8))
)
from prepared_payment;

commit;
