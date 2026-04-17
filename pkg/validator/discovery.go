package validator

// ─────────────────────────────────────────────────────────────────────────────
// Embedded GCP Discovery Rules
//
// Each entry represents a critical creation/mutation endpoint.
// Read/list/delete endpoints are intentionally less strict (they have no body).
// Rules are derived from the following Discovery Documents:
//   compute  v1  → https://discovery.googleapis.com/discovery/v1/apis/compute/v1/rest
//   sqladmin v1  → https://discovery.googleapis.com/discovery/v1/apis/sqladmin/v1/rest
//   iam      v1  → https://discovery.googleapis.com/discovery/v1/apis/iam/v1/rest
//   bigquery v2  → https://discovery.googleapis.com/discovery/v1/apis/bigquery/v2/rest
//   container v1 → https://discovery.googleapis.com/discovery/v1/apis/container/v1/rest
//   dataproc  v1 → https://discovery.googleapis.com/discovery/v1/apis/dataproc/v1/rest
//   dns       v1 → https://discovery.googleapis.com/discovery/v1/apis/dns/v1/rest
//   run       v2 → https://discovery.googleapis.com/discovery/v1/apis/run/v2/rest
//   cloudfunctions v2 → https://discovery.googleapis.com/discovery/v1/apis/cloudfunctions/v2/rest
// ─────────────────────────────────────────────────────────────────────────────

// embeddedRules is the full set of Discovery-Doc-derived validation rules.
// One entry per service domain.
var embeddedRules = []ServiceSchema{

	// ── Compute Engine ──────────────────────────────────────────────────────
	{
		Domain: "compute.googleapis.com",
		Methods: []MethodSchema{

			// instances.insert — requires `name` in the body
			{
				HTTPMethod:  "POST",
				PathGlob:    "/compute/v1/projects/*/zones/*/instances",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for instances.insert"},
				},
			},

			// networks.insert — requires `name`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/compute/v1/projects/*/global/networks",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for networks.insert"},
				},
			},

			// securityPolicies.insert — requires `name`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/compute/v1/projects/*/global/securityPolicies",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for securityPolicies.insert"},
				},
			},
		},
	},

	// ── Cloud SQL (sqladmin) ─────────────────────────────────────────────────
	{
		Domain: "sqladmin.googleapis.com",
		Methods: []MethodSchema{

			// sql.instances.insert — requires `name`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/instances",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for sql.instances.insert"},
				},
			},

			// sql.databases.insert — requires `name`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/instances/*/databases",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for sql.databases.insert"},
				},
			},

			// sql.users.insert — requires `name`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/instances/*/users",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for sql.users.insert"},
				},
			},
		},
	},

	// ── IAM ─────────────────────────────────────────────────────────────────
	{
		Domain: "iam.googleapis.com",
		Methods: []MethodSchema{

			// serviceAccounts.create — requires `accountId`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/serviceAccounts",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "accountId", Type: "string",
						Message: "field 'accountId' is required for serviceAccounts.create"},
				},
			},
		},
	},

	// ── BigQuery ─────────────────────────────────────────────────────────────
	{
		Domain: "bigquery.googleapis.com",
		Methods: []MethodSchema{

			// datasets.insert — requires datasetReference.datasetId
			{
				HTTPMethod:  "POST",
				PathGlob:    "/bigquery/v2/projects/*/datasets",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "datasetReference.datasetId", Type: "string",
						Message: "field 'datasetReference.datasetId' is required for datasets.insert"},
				},
			},

			// tables.insert — requires tableReference.tableId
			{
				HTTPMethod:  "POST",
				PathGlob:    "/bigquery/v2/projects/*/datasets/*/tables",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "tableReference.tableId", Type: "string",
						Message: "field 'tableReference.tableId' is required for tables.insert"},
				},
			},

			// jobs.insert — requires configuration.jobType
			{
				HTTPMethod:  "POST",
				PathGlob:    "/bigquery/v2/projects/*/jobs",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "configuration", Type: "object",
						Message: "field 'configuration' is required for jobs.insert"},
				},
			},

			// tabledata.insertAll — requires `rows`
			{
				HTTPMethod:  "POST",
				PathGlob:    "/bigquery/v2/projects/*/datasets/*/tables/*/insertAll",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "rows", Type: "array",
						Message: "field 'rows' is required for tabledata.insertAll"},
				},
			},
		},
	},

	// ── GKE (container) ──────────────────────────────────────────────────────
	{
		Domain: "container.googleapis.com",
		Methods: []MethodSchema{

			// clusters.create (zone-based) — requires cluster.name
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/zones/*/clusters",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "cluster.name", Type: "string",
						Message: "field 'cluster.name' is required for clusters.create"},
				},
			},

			// clusters.create (location-based) — same requirement
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/locations/*/clusters",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "cluster.name", Type: "string",
						Message: "field 'cluster.name' is required for clusters.create"},
				},
			},
		},
	},

	// ── Dataproc ─────────────────────────────────────────────────────────────
	{
		Domain: "dataproc.googleapis.com",
		Methods: []MethodSchema{

			// clusters.create — requires clusterName
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/regions/*/clusters",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "clusterName", Type: "string",
						Message: "field 'clusterName' is required for clusters.create"},
				},
			},

			// jobs.submit — requires placement.clusterName
			{
				HTTPMethod:  "POST",
				PathGlob:    "/v1/projects/*/regions/*/jobs",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "job.placement.clusterName", Type: "string",
						Message: "field 'job.placement.clusterName' is required for jobs.submit"},
				},
			},
		},
	},

	// ── Cloud DNS ────────────────────────────────────────────────────────────
	{
		Domain: "dns.googleapis.com",
		Methods: []MethodSchema{

			// managedZones.create — requires name and dnsName
			{
				HTTPMethod:  "POST",
				PathGlob:    "/dns/v1/projects/*/managedZones",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' is required for managedZones.create"},
					{Path: "dnsName", Type: "string",
						Message: "field 'dnsName' is required for managedZones.create"},
				},
			},

			// resourceRecordSets.create — requires name and type
			{
				HTTPMethod:  "POST",
				PathGlob:    "/dns/v1/projects/*/managedZones/*/rrsets",
				ContentType: "application/json",
				RequiredBody: []BodyField{
					{Path: "name", Type: "string",
						Message: "field 'name' (FQDN) is required for resourceRecordSets.create"},
					{Path: "type", Type: "string",
						Message: "field 'type' (e.g. A, CNAME, MX) is required for resourceRecordSets.create"},
				},
			},

			// changes.create — requires at least additions or deletions
			{
				HTTPMethod:  "POST",
				PathGlob:    "/dns/v1/projects/*/managedZones/*/changes",
				ContentType: "application/json",
				// No specific body field required — either additions or deletions must be present,
				// but the DNS shim handles that logic itself.
			},
		},
	},

	// ── Cloud Functions ───────────────────────────────────────────────────────
	{
		Domain: "cloudfunctions.googleapis.com",
		Methods: []MethodSchema{

			// functions.create — requires functionId query param
			{
				HTTPMethod:    "POST",
				PathGlob:      "/v2/projects/*/locations/*/functions",
				ContentType:   "application/json",
				RequiredQuery: []string{"functionId"},
			},
		},
	},

	// ── Cloud Run ─────────────────────────────────────────────────────────────
	{
		Domain: "run.googleapis.com",
		Methods: []MethodSchema{

			// services.create — requires serviceId query param
			{
				HTTPMethod:    "POST",
				PathGlob:      "/v2/projects/*/locations/*/services",
				ContentType:   "application/json",
				RequiredQuery: []string{"serviceId"},
			},
		},
	},
}
