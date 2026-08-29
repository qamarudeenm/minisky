-- Typed, deduplicated order lines with revenue measures computed once, here,
-- so every downstream mart agrees on what "revenue" means.
with source as (

    select * from {{ source('retail_raw', 'orders') }}

),

deduplicated as (

    select
        order_id,
        customer_id,
        order_date,
        lower(trim(status)) as order_status,
        lower(trim(coalesce(payment_method, 'unknown'))) as payment_method,
        quantity,
        unit_price,
        coalesce(discount, 0) as discount_rate,
        loaded_at,
        row_number() over (partition by order_id order by loaded_at desc) as recency_rank
    from source

)

select
    order_id,
    customer_id,
    order_date,
    order_status,
    payment_method,
    quantity,
    unit_price,
    discount_rate,
    round(quantity * unit_price, 2) as gross_revenue,
    round(quantity * unit_price * (1 - discount_rate), 2) as net_revenue,
    order_status in ('cancelled', 'returned') as is_lost_order,
    loaded_at
from deduplicated
where recency_rank = 1
