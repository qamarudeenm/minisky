-- One clean row per customer, with the source's free-text segment normalised.
with source as (

    select * from {{ source('retail_raw', 'customers') }}

),

deduplicated as (

    select
        customer_id,
        trim(full_name) as full_name,
        lower(trim(coalesce(email, ''))) as email,
        trim(coalesce(country, 'unknown')) as country,
        signup_date,
        lower(trim(coalesce(segment, 'consumer'))) as customer_segment,
        loaded_at,
        row_number() over (partition by customer_id order by loaded_at desc) as recency_rank
    from source

)

select
    customer_id,
    full_name,
    email,
    country,
    signup_date,
    customer_segment,
    loaded_at
from deduplicated
where recency_rank = 1
