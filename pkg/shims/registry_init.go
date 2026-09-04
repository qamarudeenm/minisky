package shims

import (
	"minisky/pkg/registry"
	
	// Blank imports to trigger init() in all shim packages
	_ "minisky/pkg/shims/appengine"
	_ "minisky/pkg/shims/artifactregistry"
	_ "minisky/pkg/shims/bigquery"
	_ "minisky/pkg/shims/bigtable"
	_ "minisky/pkg/shims/cloudsql"
	_ "minisky/pkg/shims/cloudkms"
	_ "minisky/pkg/shims/cloudtasks"
	_ "minisky/pkg/shims/cloudbuild"
	_ "minisky/pkg/shims/compute"
	_ "minisky/pkg/shims/dataproc"
	_ "minisky/pkg/shims/dns"
	_ "minisky/pkg/shims/firebaseauth"
	_ "minisky/pkg/shims/firebasehosting"
	_ "minisky/pkg/shims/firebasertdb"
	_ "minisky/pkg/shims/gke"
	_ "minisky/pkg/shims/iam"
	_ "minisky/pkg/shims/logging"
	_ "minisky/pkg/shims/memorystore"
	_ "minisky/pkg/shims/metadata"
	_ "minisky/pkg/shims/monitoring"
	_ "minisky/pkg/shims/pubsub"
	_ "minisky/pkg/shims/scheduler"
	_ "minisky/pkg/shims/secretmanager"
	_ "minisky/pkg/shims/serverless"
	_ "minisky/pkg/shims/storage"
	_ "minisky/pkg/shims/vertexai"
)

func init() {
	// Register services that don't have a custom Go shim but use direct Docker emulators
	registry.RegisterLazyDocker("firestore.googleapis.com")
	registry.RegisterLazyDocker("datastore.googleapis.com")
	registry.RegisterLazyDocker("spanner.googleapis.com")

	// Docker-backed services declare their routes here rather than in a shim
	// init(), because they have no Go shim to hold the declaration. Clients
	// reach them through the gateway the same way, so they still need a route.
	registry.RegisterRoutes("spanner.googleapis.com",
		"/v1/projects/*/instances",
		"/v1/projects/*/instanceConfigs",
	)

	registry.RegisterRoutes("firestore.googleapis.com",
		"/v1/projects/*/databases",
	)

	// Datastore puts its custom methods on the project itself, as
	// /v1/projects/{projectId}:runQuery, so each verb is named explicitly.
	// "/v1/projects/*" would match, but it would also swallow every other
	// service sharing that prefix.
	registry.RegisterRoutes("datastore.googleapis.com",
		"/v1/projects/*:runQuery",
		"/v1/projects/*:runAggregationQuery",
		"/v1/projects/*:lookup",
		"/v1/projects/*:commit",
		"/v1/projects/*:beginTransaction",
		"/v1/projects/*:rollback",
		"/v1/projects/*:allocateIds",
		"/v1/projects/*:reserveIds",
	)
}
