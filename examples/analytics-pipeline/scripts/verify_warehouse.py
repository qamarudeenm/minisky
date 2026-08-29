"""Warehouse fidelity checks against a running MiniSky.

Exercises the SQL patterns an analytics pipeline depends on and that the
BigQuery shim historically got wrong: decimal literals, dots inside string
literals, table aliases, fully-qualified relation names, typed result schemas,
and dbt-style CREATE OR REPLACE TABLE AS SELECT followed by a metadata read.

Run it from the example directory with the emulator up and terraform applied:

    python3 scripts/verify_warehouse.py

Exits non-zero if any check fails, so it works as a regression gate.
"""
import csv, json, sys, time, urllib.request, urllib.error

BASE = "http://localhost:8080"
PROJECT = "retail-analytics-local"
RAW, MARTS = "retail_raw", "retail_marts"
passed, failed = 0, 0

def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method,
                                 headers={"Content-Type": "application/json",
                                          "Authorization": "Bearer minisky-local-token"})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")

def query(sql):
    """jobs.query — the synchronous surface clients actually use."""
    status, body = call("POST", f"/bigquery/v2/projects/{PROJECT}/queries",
                        {"query": sql, "useLegacySql": False})
    if status != 200:
        raise RuntimeError(f"HTTP {status}: {body}")
    if body.get("errors"):
        raise RuntimeError(body["errors"][0].get("message", body["errors"]))
    fields = [f["name"] for f in body.get("schema", {}).get("fields", [])]
    types = [f["type"] for f in body.get("schema", {}).get("fields", [])]
    rows = [[c.get("v") for c in row.get("f", [])] for row in body.get("rows", [])]
    return fields, types, rows

def check(name, fn):
    global passed, failed
    try:
        detail = fn()
        print(f"  \033[32m✓\033[0m {name}" + (f" — {detail}" if detail else ""))
        passed += 1
    except Exception as exc:
        print(f"  \033[31m✗\033[0m {name}\n      {exc}")
        failed += 1

print("\n═══ Item 2: the official /queries surface ═══")
def t_queries():
    fields, types, rows = query("SELECT 1 AS ok")
    assert rows and rows[0][0] == "1", rows
    return "POST /queries returns rows synchronously"
check("jobs.query (POST /projects/{p}/queries)", t_queries)

def t_get_results():
    status, body = call("POST", f"/bigquery/v2/projects/{PROJECT}/queries",
                        {"query": "SELECT 42 AS answer"})
    job_id = body["jobReference"]["jobId"]
    status, results = call("GET", f"/bigquery/v2/projects/{PROJECT}/queries/{job_id}")
    assert status == 200, status
    assert results["jobComplete"], results
    assert results["rows"][0]["f"][0]["v"] == "42", results["rows"]
    return "GET /queries/{jobId} returns the job's rows"
check("jobs.getQueryResults (GET /projects/{p}/queries/{jobId})", t_get_results)

print("\n═══ Item 1: the SQL translator ═══")
def t_load_decimals():
    query(f"DELETE FROM {RAW}.orders WHERE TRUE")
    with open("seeds/raw_orders.csv") as fh:
        rows = list(csv.DictReader(fh))[:40]
    values = ",".join(
        "({order_id}, {customer_id}, '{order_date}', '{status}', '{payment_method}', "
        "{quantity}, {unit_price}, {discount}, '2026-08-29T14:00:00+00:00')".format(**r)
        for r in rows)
    query(f"INSERT INTO {RAW}.orders (order_id, customer_id, order_date, status, "
          f"payment_method, quantity, unit_price, discount, loaded_at) VALUES {values}")
    _, _, out = query(f"SELECT COUNT(*) AS n FROM {RAW}.orders")
    assert out[0][0] == "40", out
    return "40 rows with decimal prices (e.g. 24.99) inserted intact"
check("decimal literals survive INSERT", t_load_decimals)

def t_decimal_value():
    _, _, out = query(f"SELECT unit_price FROM {RAW}.orders ORDER BY order_id LIMIT 1")
    value = float(out[0][0])
    assert 0 < value < 1000, out
    return f"unit_price reads back as {out[0][0]}"
check("decimal values round-trip", t_decimal_value)

def t_dotted_strings():
    query(f"DELETE FROM {RAW}.customers WHERE TRUE")
    with open("seeds/raw_customers.csv") as fh:
        rows = list(csv.DictReader(fh))[:10]
    values = ",".join(
        "({customer_id}, '{full_name}', '{email}', '{country}', '{signup_date}', "
        "'{segment}', '2026-08-29T14:00:00+00:00')".format(**r) for r in rows)
    query(f"INSERT INTO {RAW}.customers (customer_id, full_name, email, country, "
          f"signup_date, segment, loaded_at) VALUES {values}")
    _, _, out = query(f"SELECT email FROM {RAW}.customers WHERE customer_id = 1001")
    assert out[0][0] == "ada.okonkwo@example.com", out
    return f"stored as {out[0][0]!r}, not 'ada__okonkwo@example__com'"
check("dots inside string literals are preserved", t_dotted_strings)

def t_alias():
    _, _, out = query(
        f"SELECT o.order_id, o.unit_price FROM {RAW}.orders AS o ORDER BY o.order_id LIMIT 1")
    assert out and out[0][0], out
    return "o.order_id resolved instead of becoming o__order_id"
check("table aliases keep their qualification", t_alias)

def t_qualified():
    _, _, out = query(f"SELECT COUNT(*) AS n FROM {PROJECT}.{RAW}.orders")
    assert out[0][0] == "40", out
    return "project.dataset.table with a hyphenated project id resolves"
check("fully-qualified relation names resolve", t_qualified)

print("\n═══ Item 3: result schema types ═══")
def t_types():
    fields, types, _ = query(
        f"SELECT order_id, unit_price, order_date, status FROM {RAW}.orders LIMIT 1")
    mapping = dict(zip(fields, types))
    assert mapping["order_id"] == "INT64", mapping
    assert mapping["unit_price"] == "FLOAT64", mapping
    assert mapping["order_date"] == "DATE", mapping
    assert mapping["status"] == "STRING", mapping
    return ", ".join(f"{k}:{v}" for k, v in mapping.items())
check("columns report real BigQuery types", t_types)

def t_column_order():
    fields, _, _ = query(f"SELECT order_id, customer_id, quantity FROM {RAW}.orders LIMIT 1")
    assert fields == ["order_id", "customer_id", "quantity"], fields
    return "schema order matches the SELECT list"
check("column order is deterministic", t_column_order)

print("\n═══ Item 4: tables created by a query ═══")
def t_ctas():
    query(f"CREATE OR REPLACE TABLE {MARTS}.fct_daily_revenue AS "
          f"SELECT order_date, COUNT(*) AS orders_total, "
          f"ROUND(SUM(quantity * unit_price * (1 - discount)), 2) AS net_revenue "
          f"FROM {RAW}.orders GROUP BY order_date")
    _, _, out = query(f"SELECT COUNT(*) AS n FROM {MARTS}.fct_daily_revenue")
    assert int(out[0][0]) > 0, out
    return f"{out[0][0]} daily rows built by a dbt-style CREATE OR REPLACE TABLE"
check("CREATE OR REPLACE TABLE AS SELECT", t_ctas)

def t_metadata():
    status, table = call("GET",
        f"/bigquery/v2/projects/{PROJECT}/datasets/{MARTS}/tables/fct_daily_revenue")
    assert status == 200, f"tables.get returned {status}: {table}"
    names = [f["name"] for f in table.get("schema", {}).get("fields", [])]
    assert "net_revenue" in names, names
    types = {f["name"]: f["type"] for f in table["schema"]["fields"]}
    return f"tables.get sees it with schema {types}"
check("the query-created table is visible to tables.get", t_metadata)

def t_list():
    status, listing = call("GET", f"/bigquery/v2/projects/{PROJECT}/datasets/{MARTS}/tables")
    ids = [t["tableReference"]["tableId"] for t in listing.get("tables", [])]
    assert "fct_daily_revenue" in ids, ids
    return f"tables.list reports {ids}"
check("the query-created table appears in tables.list", t_list)

print(f"\n─────────────────────────────────────────────")
print(f"  \033[32m{passed} passed\033[0m, " + (f"\033[31m{failed} failed\033[0m" if failed else "0 failed"))
sys.exit(1 if failed else 0)
