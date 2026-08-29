-- Customer dimension enriched with lifetime order behaviour.
-- Joins use USING(...) so no column needs table-qualification.
with customers as (

    select * from {{ ref('stg_customers') }}

),

order_stats as (

    select
        customer_id,
        count(*) as lifetime_orders,
        round(sum(net_revenue), 2) as lifetime_net_revenue,
        min(order_date) as first_order_date,
        max(order_date) as latest_order_date
    from {{ ref('stg_orders') }}
    where not is_lost_order
    group by customer_id

)

select
    customer_id,
    full_name,
    email,
    country,
    customer_segment,
    signup_date,
    coalesce(lifetime_orders, 0) as lifetime_orders,
    coalesce(lifetime_net_revenue, 0) as lifetime_net_revenue,
    first_order_date,
    latest_order_date
from customers
left join order_stats using (customer_id)
