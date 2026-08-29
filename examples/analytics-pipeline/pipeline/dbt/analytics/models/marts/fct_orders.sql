-- Order fact at its natural grain: one row per order, with the customer
-- attributes an analyst needs to slice it without a second join.
with orders as (

    select * from {{ ref('stg_orders') }}

),

customers as (

    select
        customer_id,
        full_name as customer_name,
        country as customer_country,
        customer_segment
    from {{ ref('stg_customers') }}

)

select
    order_id,
    customer_id,
    customer_name,
    customer_country,
    customer_segment,
    order_date,
    order_status,
    payment_method,
    quantity,
    unit_price,
    discount_rate,
    gross_revenue,
    net_revenue,
    is_lost_order
from orders
left join customers using (customer_id)
