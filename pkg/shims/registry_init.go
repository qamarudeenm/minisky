package shims

import (
	"minisky/pkg/registry"
	
	// Blank imports to trigger init() in all shim packages
	_ "minisky/pkg/shims/appengine"
	_ "minisky/pkg/shims/bigquery"
	_ "minisky/pkg/shims/bigtable"
	_ "minisky/pkg/shims/cloudsql"
	_ "minisky/pkg/shims/cloudtasks"
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
)

func init() {
	// Register services that don't have a custom Go shim but use direct Docker emulators
	registry.RegisterLazyDocker("firestore.googleapis.com")
	registry.RegisterLazyDocker("datastore.googleapis.com")
	registry.RegisterLazyDocker("spanner.googleapis.com")
}
