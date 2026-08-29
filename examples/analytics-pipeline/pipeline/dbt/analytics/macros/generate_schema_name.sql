{#
  Terraform owns the dataset names, so a model's +schema is used verbatim
  instead of dbt's default "<target dataset>_<custom schema>" concatenation.
#}
{% macro generate_schema_name(custom_schema_name, node) -%}
    {%- if custom_schema_name is none -%}
        {{ target.schema }}
    {%- else -%}
        {{ custom_schema_name | trim }}
    {%- endif -%}
{%- endmacro %}
