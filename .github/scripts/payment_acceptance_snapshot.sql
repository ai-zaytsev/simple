with matches as (
    select a.id, a.tier, a.vip_expires_at
    from accounts a
    where a.id::text like :'prefix' || '%'
), selected as (
    select * from matches where (select count(*) from matches) = 1
), latest_payment as (
    select p.*
    from payments p
    where p.account_id = (select id from selected)
    order by p.created_at desc
    limit 1
), latest_refund as (
    select r.*
    from refunds r
    where r.payment_id = (select id from latest_payment)
    order by r.created_at desc
    limit 1
)
select jsonb_build_object(
    'match_count', (select count(*) from matches),
    'account', (
        select jsonb_build_object(
            'id', id,
            'tier', tier,
            'vip_expires_at', vip_expires_at
        ) from selected
    ),
    'payment', (
        select jsonb_build_object(
            'id', p.id,
            'product_id', p.product_id,
            'provider', p.provider,
            'provider_payment_id', p.provider_payment_id,
            'amount_minor', p.amount_minor,
            'currency', p.currency,
            'status', p.status,
            'provider_test', p.provider_test,
            'payment_method', p.payment_method,
            'provider_refundable', p.provider_refundable,
            'created_at', p.created_at,
            'paid_at', p.paid_at,
            'entitlement_applied_at', p.entitlement_applied_at,
            'entitlement_started_at', p.entitlement_started_at,
            'entitlement_ends_at', p.entitlement_ends_at
        ) from latest_payment p
    ),
    'refund', (
        select jsonb_build_object(
            'id', r.id,
            'payment_id', r.payment_id,
            'provider', r.provider,
            'amount_minor', r.amount_minor,
            'currency', r.currency,
            'mode', r.mode,
            'status', r.status,
            'cancellation_reason', r.cancellation_reason,
            'calculated_at', r.calculated_at,
            'succeeded_at', r.succeeded_at,
            'entitlement_revoked_at', r.entitlement_revoked_at,
            'attempt', (
                select jsonb_build_object(
                    'attempt_no', ra.attempt_no,
                    'provider_refund_id', ra.provider_refund_id,
                    'status', ra.status,
                    'cancellation_reason', ra.cancellation_reason
                )
                from refund_attempts ra
                where ra.refund_id = r.id
                order by ra.attempt_no desc
                limit 1
            )
        ) from latest_refund r
    ),
    'refund_count', (
        select count(*) from refunds r
        where r.payment_id = (select id from latest_payment)
    ),
    'succeeded_refund_count', (
        select count(*) from refunds r
        where r.payment_id = (select id from latest_payment)
          and r.status = 'succeeded'
    ),
    'succeeded_refund_total_minor', (
        select coalesce(sum(r.amount_minor), 0) from refunds r
        where r.payment_id = (select id from latest_payment)
          and r.status = 'succeeded'
    ),
    'refund_attempt_count', (
        select count(*)
        from refund_attempts ra
        join refunds r on r.id = ra.refund_id
        where r.payment_id = (select id from latest_payment)
    )
);
