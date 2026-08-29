-- The headline mart: revenue per day, with cancelled and returned orders
-- excluded from revenue but still counted so the loss is visible.
with orders as (

    select * from {{ ref('stg_orders') }}

)

select
    order_date,
    count(*) as orders_total,
    sum(case when is_lost_order then 1 else 0 end) as orders_lost,
    count(distinct customer_id) as unique_customers,
    sum(quantity) as units_sold,
    round(sum(case when is_lost_order then 0 else gross_revenue end), 2) as gross_revenue,
    round(sum(case when is_lost_order then 0 else net_revenue end), 2) as net_revenue,
    round(
        sum(case when is_lost_order then 0 else net_revenue end)
        / nullif(sum(case when is_lost_order then 0 else 1 end), 0),
        2
    ) as avg_order_value
from orders
group by order_date
