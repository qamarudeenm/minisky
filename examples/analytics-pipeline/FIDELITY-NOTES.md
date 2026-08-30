# Fidelity notes — what this pipeline learned about MiniSky v1.3.1

Building a real analytics pipeline against the emulator is a benchmark of the emulator itself.
These are the gaps found while writing this example, with the evidence and the fix each one needs.
Nothing here is speculative: every item was read out of the shim source.

Severity key: **blocking** = a normal pipeline cannot work around it · **high** = breaks a common
tool · **medium** = surprising but workable · **low** = documented behaviour worth calling out.

**Status: items 1, 2, 3, 4, 6 and 11 through 19 are now fixed in this repository.** Each fix carries Go unit
tests (`pkg/shims/bigquery/sqltranslate_test.go`, `pkg/shims/bigquery/query_surface_test.go`,
`pkg/shims/compute/api_test.go`) and is verified live against a running emulator by
`scripts/verify_warehouse.py`. Items 5, 7, 8, 9, 10 and 20 remain open.

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

### 13. ~~Every `terraform apply` destroyed and recreated the buckets~~ — **fixed**

Found by applying twice and reading the second plan:

```
# google_storage_bucket.raw must be replaced
  ~ location = "US-CENTRAL1" -> "US" # forces replacement
```

fake-gcs-server does not model a bucket's location, labels or storage class: it accepts them on
create and reports its own defaults. Because `location` is ForceNew in the provider, a bucket
created with `location = "US"` was destroyed and recreated on **every** apply — taking every
object in it. A data lake could not survive a second `terraform apply`.

**Fixed in `pkg/shims/storage/metadata.go`:** the shim records the location, storage class and
labels from bucket insert/patch requests and overlays them onto the emulator's responses, for both
a single bucket and a bucket listing. Responses for buckets it has not seen pass through
untouched, and the registry is dropped on delete. Covered by `metadata_test.go`.

The registry is in-memory, so buckets created before a restart report the emulator's default again
until they are re-created — the same limitation as item 15.

### 14. ~~Instance updates returned 405, and the shim overwrote the caller's description~~ — **fixed**

Two defects compounding each other. The compute shim wrote its container mapping straight into the
instance's `description`:

```go
i.Description = fmt.Sprintf("Docker Container ID mapping: %s", containerName)
```

so an instance created with a description came back with a different one, and terraform saw drift
on every plan. It then tried to reconcile that drift and got `405` — `instances.update` /
`instances.patch` had no handler at all, which failed the apply outright.

**Fixed:** the container mapping moves to a `minisky-container` label, a description is only
synthesised when the caller supplied none, and `PATCH`/`PUT` on an instance now applies
description, labels and metadata (preserving the shim-owned mapping label). Covered by
`TestSetContainerMappingPreservesDescription` and `TestUpdateInstanceAppliesMutableFields`.

### 15. ~~Firewall rules never published a single port~~ — **fixed**

The headline feature of `docs/network-firewall.md` — "Level 2: Firewall Port Binding" — did not
work at all. Two places decided whether a rule permits traffic by reading `rule.Action`:

```go
if ... && rule.Direction == "INGRESS" && rule.Action == "allow" {
```

A GCE firewall has **no `action` field**. Allow versus deny is expressed by which of `allowed` /
`denied` is populated, and the ports live inside those entries. So `Action` was always `""`, the
condition never matched, and every VM was provisioned with `ports: 0`:

```
[Orchestrator] Provisioning compute VM: minisky-vm-... (... ports: 0 ...)
```

The registration path had the same defect from the other side — it stored `Ports: []string{}` with
a comment reading `// default, will refine below`, which nothing ever did. The net effect: nothing
running inside an emulated VM was ever reachable from the host, and the failure was silent.

**Fixed:** `firewallEffect` derives action, protocol and ports from the `allowed`/`denied` entries
(an explicitly declared action still wins, for dashboard-created rules), port ranges are expanded
with a cap so `0-65535` cannot try to bind every port on the host, and `direction` defaults to
`INGRESS` as GCE does. After the fix the same VM provisions with `ports: 2` and Docker publishes
them. Covered by `TestFirewallEffectReadsAllowedEntries`,
`TestGetAllowedPortsForVPCReadsCreatedRules`, `TestGetAllowedPortsForVPCIgnoresDeniedAndDisabled`
and `TestExpandPortRange`.

### 16. ~~`instances.insert` silently adopted a pre-existing container~~ — **fixed**

`ProvisionComputeVM` treats Docker's `409 Conflict` on container create as success and starts the
existing container instead. The requested port bindings and boot image are then never applied,
because Docker can only set those at create time. The API still reports the instance as freshly
`RUNNING`, so Terraform believes it created a new VM while it actually inherited a stale one — with
no published ports, which is exactly the symptom `scripts/pipeline_status.sh` warns about:

```
! no host port published for container port 8080
```

It is reachable in normal use: restart MiniSky (which empties the in-memory instance registry),
re-apply, and the VM comes back portless. Real GCE returns `409 ALREADY_EXISTS` for a duplicate
instance name rather than adopting anything.

**Fixed:** `ProvisionComputeVM` now removes the conflicting container and creates it again, so the
requested image and port bindings actually take effect. Covered by
`TestProvisionComputeVM_ReplacesExistingContainer`.

### 17. ~~A VM had no network route to the emulator~~ — **fixed**

`ProvisionComputeVM` created VM containers with no `ExtraHosts`, so from inside an emulated VM the
gateway was unreachable and `host.docker.internal` did not resolve:

```
✗ host.docker.internal unreachable
✗ 172.17.0.1 unreachable
✗ 172.21.0.1 unreachable      # the VPC network's own gateway
```

On Docker Desktop the daemon runs inside its own VM, so a container's bridge gateway is not the
host. The consequence is total: nothing running inside an emulated VM — the point of having VMs —
could call a single emulated GCP API.

**Fixed:** every VM is now created with `host.docker.internal:host-gateway`, the same name Docker
Desktop users already reach the host by. Covered by
`TestProvisionComputeVM_GivesVMsARouteToTheHost`.

### 18. ~~Path-based routing only worked for `Host: localhost`~~ — **fixed**

Even with a route, requests from a VM were rejected. The router only path-mapped when the Host
header contained `localhost` or `127.0.0.1`:

```go
if strings.Contains(targetDomain, "localhost") || strings.Contains(targetDomain, "127.0.0.1") {
```

A client inside a VM connects to `host.docker.internal:8080` (or an IP), so its Host header matched
neither, `targetDomain` stayed `host.docker.internal`, no shim was registered under that name, and
every request came back:

```json
{"error":{"code":501,"message":"MiniSky: 'host.docker.internal' is not yet implemented"}}
```

The same applied to any host that is not literally localhost — a LAN address, a container name, a
custom DNS entry.

**Fixed:** the gate is inverted. A request whose Host names a Google API domain
(`*.googleapis.com`, `*.firebaseio.com`, `*.google.internal`) still routes by domain; everything
else — which is every client addressing the emulator directly — routes by URL prefix. Covered by
`pkg/router/proxy_test.go`.

### 19. ~~No dbt model could be materialised: `OPTIONS(...)` was not translated~~ — **fixed**

The last thing standing between dbt and a working pipeline. Every dbt materialisation emits
BigQuery's `OPTIONS` clause:

```sql
create or replace table `p`.`retail_staging`.`stg_orders`
  OPTIONS(description="", expiration_timestamp=NULL)
as (select ...)
```

DuckDB has no such clause, so the whole statement failed and `dbt build` reported:

```
Database Error in model stg_orders (models/staging/stg_orders.sql)
  500 Parser Error: syntax error at or near "OPTIONS"
```

**Fixed in `pkg/shims/bigquery/sqltranslate.go`:** `stripOptionsClauses` removes the clause from
DDL before the statement reaches DuckDB. The options are pure metadata, so the result is
unchanged. Only DDL is touched, the parenthesis group is matched with a depth counter so nested
parens and parens inside strings do not end it early, an unbalanced clause is left alone rather
than truncating the statement, and a column or alias named `options` in a query is untouched.
Covered by `TestStripsOptionsClauseFromDDL`, `TestLeavesOptionsAloneOutsideDDL` and
`TestUnbalancedOptionsClauseIsLeftAlone`.

### 20. Dataset and table metadata does not survive a restart — **medium, open**

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
`INSERT` rather than a load job), item 10 (`depends_on` orders firewall rules before the VM),
and item 20 (re-apply after a restart).

Items 11 through 19 were found by the exercise itself rather than by reading the source — by
running `terraform apply` twice and reading the second plan, then by pushing an actual pipeline
through the stack. Most were silent: the apply reported success every time while never converging,
item 13 was quietly destroying the data lake on each run, and items 15, 17 and 18 together meant
nothing running inside an emulated VM could reach the emulator at all.

With all of them fixed the pipeline runs end to end against MiniSky: `verify_infra.sh --data`
reports **41 passed, 0 failed**, and `fct_daily_revenue` holds 30 days of revenue built by dbt
from seed CSVs in the emulated data lake.
