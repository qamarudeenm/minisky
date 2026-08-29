# Fidelity notes — what this pipeline learned about MiniSky v1.3.1

Building a real analytics pipeline against the emulator is a benchmark of the emulator itself.
These are the gaps found while writing this example, with the evidence and the fix each one needs.
Nothing here is speculative: every item was read out of the shim source.

Severity key: **blocking** = a normal pipeline cannot work around it · **high** = breaks a common
tool · **medium** = surprising but workable · **low** = documented behaviour worth calling out.

**Status: items 1, 2, 3, 4, 6, 11 and 12 are now fixed in this repository.** Each fix carries Go unit
tests (`pkg/shims/bigquery/sqltranslate_test.go`, `pkg/shims/bigquery/query_surface_test.go`,
`pkg/shims/compute/api_test.go`) and is verified live against a running emulator by
`scripts/verify_warehouse.py`. Items 5, 7, 8, 9, 10 and 13 remain open.

---

### 1. ~~The BigQuery→DuckDB SQL translator rewrites dotted tokens everywhere~~ — **fixed**

`pkg/shims/bigquery/duckdb_integration_cgo.go` (`translateBQtoDuck`) rewrites
`dataset.table` → `dataset__table` with two bare regexes over the whole statement:

```go
projectRe := regexp.MustCompile(`([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
datasetRe := regexp.MustCompile(`([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
```

Neither is aware of string literals, numeric literals or column qualification, so:

| Input SQL | Sent to DuckDB | Result |
|---|---|---|
| `VALUES (1, 24.99)` | `VALUES (1, 24__99)` | syntax error |
| `WHERE email = 'ada.okonkwo@example.com'` | `'ada__okonkwo@example__com'` | silent data corruption |
| `SELECT o.order_id FROM ... o` | `SELECT o__order_id` | column not found |

Any revenue pipeline has decimal literals, so this is the one gap with no client-side workaround.

**Fixed in `pkg/shims/bigquery/sqltranslate.go`:** the statement is now scanned rather than
regex-replaced. String literals, quoted identifiers, comments and numeric literals are copied
through verbatim; a two-part chain is collapsed only when its first segment names a dataset the
shim knows (registered by `datasets.insert`, plus the prefixes of tables already in DuckDB), so
`alias.column` survives while `dataset.table` resolves. Three-part chains stay resolvable even
with an empty registry, which keeps queries working after a restart.

### 2. ~~`getQueryResults` is served on a non-standard path~~ — **fixed**

`pkg/shims/bigquery/api.go` routes results at `/projects/{p}/jobs/{jobId}/results`. The real REST
surface is `GET /projects/{p}/queries/{jobId}` (and `POST /projects/{p}/queries` for the
jobs.query fast path). `google-cloud-bigquery` — and therefore dbt, `bq`, and every SDK — calls
`/queries/...`, which falls through to the `default:` branch and 404s.

**Fixed in `pkg/shims/bigquery/api.go`:** `routeQueries` now serves both
`POST /projects/{p}/queries` (jobs.query, executed synchronously) and
`GET /projects/{p}/queries/{jobId}` (jobs.getQueryResults). The original
`/jobs/{jobId}/results` path still works, so nothing written against it breaks. The client-side
path rewrite the example used to carry has been removed.

### 3. ~~Query result schemas are flattened to STRING~~ — **fixed**

`insertJob` builds the response schema from the first result row with `Type: "STRING"` for every
column, and `getQueryResults` renders every value with `fmt.Sprintf("%v", ...)`. Numeric and
timestamp columns arrive at the client as strings, so client-side aggregation and any adapter that
trusts the declared type gets the wrong Python/Go type.

**Fixed:** `ExecuteQueryWithSchema` returns an ordered, typed column list built from
`rows.ColumnTypes()`, mapped to BigQuery types by `duckToBQType`. Values are rendered by
`formatCell` in the shapes the REST API uses — DATE as `2026-07-01`, TIMESTAMP as epoch seconds,
NULL as `null` rather than the string `<nil>`. Column order is now deterministic too: the schema
used to be built by ranging over a map, so the field order changed between identical queries.

### 4. ~~Tables created by a query are invisible to the metadata API~~ — **fixed**

`CREATE TABLE AS` executed through a query job lands in DuckDB but is never registered in the
shim's `tables` map, so `tables.get` / `tables.list` 404 for it. dbt creates every model this way
and then reads the relation back through the metadata API.

**Fixed:** after a query job succeeds, `reconcileTables` reads DuckDB's `information_schema` and
registers any table the metadata map has not seen (refreshing the schema of ones it has), so
`tables.get` and `tables.list` find dbt-created models. `tables.delete` now also drops the
underlying DuckDB table, which previously left data behind that queries could still read.

### 5. `metadata.startup-script` is stored but never executed — **medium**

`pkg/shims/compute/api.go` provisions the VM container with `[]string{"tail", "-f", "/dev/null"}`
and there is no guest agent, so the single most common way to bootstrap a GCE VM does nothing.
This example ships its `startup-script` in metadata for portability and delivers the same work over
`docker exec` (`provision.tf`).

**Fix:** when `metadata.items` contains `startup-script`, write it into the container and run it
after start — a real guest-agent emulation in ~20 lines.

### 6. ~~`boot_disk.initialize_params.image` is ignored~~ — **fixed**

`AttachedDisk` has no `initializeParams` field, so the shim only ever sees `disks[].source`, falls
back to `ubuntu:26.04`, and silently ignores `image = "ubuntu-2204-lts"` — even though
`pkg/config/images.json` already maps every `os_images` id to a Docker image.

**Fixed in `pkg/shims/compute/api.go`:** `AttachedDisk` now models `initializeParams`, and
`resolveOsImage` maps a source image — a family id, a full
`projects/…/global/images/family/…` path, or a raw Docker reference — onto the registry's image.
`diskSizeGb` is deliberately left undeclared: the API types it as a string but clients differ, and
decoding it strictly would reject otherwise valid requests.

### 7. No `/regions/*/subnetworks` route — **medium**

`google_compute_subnetwork` 404s, so custom-mode VPCs cannot be expressed. This example uses
`auto_create_subnetworks = true` to stay honest about that.

### 8. Cloud SQL exists as a shim but is unreachable from Terraform — **medium**

`pkg/router/proxy.go` path-maps only `/storage/`, `/bigquery/`, `/compute/`, `/v2/` and the Pub/Sub
paths. A `sql_custom_endpoint` pointing at `/sql/v1beta4/` never resolves to
`sqladmin.googleapis.com` and returns 501, even though the shim provisions a real Postgres
container. That is why the serving layer here is BigQuery rather than Cloud SQL.

**Fix:** one more `else if strings.HasPrefix(path, "/sql/")` branch in the router.

### 9. BigQuery load jobs cannot read `gs://` URIs — **medium**

`DuckDBBackend.LoadData` passes `sourceUris[0]` straight into `read_csv_auto('...')`, so a load job
from the emulated Cloud Storage fails — DuckDB has no GCS reader configured. The DAG therefore
ingests with batched `INSERT`s instead of a load job.

**Fix:** resolve `gs://bucket/object` through the storage shim to a local path before handing it to
DuckDB.

### 10. A firewall change re-provisions every VM on the network — **low, by design**

Documented in `docs/network-firewall.md` (Level 2): port bindings can only be set at container
create time, so adding a rule tears the VM down and rebuilds it — discarding anything installed on
it. This example works with the behaviour by making the VM `depends_on` its firewall rules.

**Worth adding to the docs:** a note that firewall rules should exist before the VMs that need
them, because the failure mode (software silently disappearing) is hard to diagnose.

### 11. ~~Metadata updates were never applied, so `terraform apply` never converged~~ — **fixed**

`friendlyName` was absent from both the `Dataset` and `Table` structs, and the dataset PATCH
handler echoed the stored resource back without writing the request:

```go
// Terraform often PATCHes datasets to update metadata/labels.
// In the shim, we just return the existing dataset as a successful update.
```

Tables had no PATCH handler at all and answered `405`. The visible effect is that every
`terraform plan` proposes the identical in-place update forever — the loop that idempotency is
supposed to close:

```
# google_bigquery_dataset.raw will be updated in-place
  + friendly_name = "retail raw"
```

**Fixed:** both resources carry `friendlyName`, dataset PATCH applies friendlyName / description /
labels / location, and tables accept PATCH (including a schema change, which is re-applied to
DuckDB). Covered by `TestPatchAppliesDatasetMetadata` and `TestPatchAppliesTableMetadata`.

### 12. ~~`google_compute_network` never converged either~~ — **fixed**

The same class of defect in the compute shim: `Network` did not model
`networkFirewallPolicyEnforcementOrder`, which GCE reports on every network (defaulting to
`AFTER_CLASSIC_FIREWALL`). Every plan proposed:

```
# google_compute_network.pipeline will be updated in-place
  + network_firewall_policy_enforcement_order = "AFTER_CLASSIC_FIREWALL"
```

**Fixed:** the field is modelled, accepted on create, defaulted to GCE's value, and reported on
the synthetic `default` network too. Covered by
`TestNetworkReportsFirewallPolicyEnforcementOrder`.

### 13. Dataset and table metadata does not survive a restart — **medium, open**

`api.datasets` and `api.tables` are in-memory only. DuckDB keeps the data, but after
`minisky restart` a `terraform plan` sees every dataset and table as missing and proposes to
recreate them. The reconciliation added for item 4 repairs the table registry after the first
query, but nothing repairs it before that, and datasets are never repaired.

**Fix:** persist the dataset/table registry alongside the DuckDB file, or rebuild it from
`information_schema` at startup the way `reconcileTables` does after a query.

---

## What this means for the benchmark

Items 1–4 were what stood between MiniSky and a working analytics pipeline, and all four are now
closed. `scripts/verify_warehouse.py` proves it against a live emulator: 40 order rows with
decimal prices load intact, `ada.okonkwo@example.com` survives a round trip, `o.order_id` resolves,
result columns report INT64/FLOAT64/DATE, and a dbt-style `CREATE OR REPLACE TABLE AS SELECT`
produces a mart that `tables.get` can then describe.

The five that remain are all workable from the client side, and each is noted where the example
works around it: item 5 (`provision.tf` uses `docker exec`), item 7 (`auto_create_subnetworks`),
item 8 (BigQuery serves as the serving layer instead of Cloud SQL), item 9 (the DAG ingests with
`INSERT` rather than a load job), item 10 (`depends_on` orders firewall rules before the VM) and
item 13 (re-apply after a restart).

Items 11 and 12 were found by the exercise itself rather than by reading the source: running
`terraform apply` twice and looking at the second plan. Both were silent — the apply reported
success every time while never converging.
