package metadata

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// instanceMetadata holds per-VM metadata injected at container start time.
// The zero value is safe and returns sensible local-dev defaults.
type instanceMetadata struct {
	ProjectID        string
	NumericProjectID string
	InstanceName     string
	InstanceID       string
	Zone             string
	MachineType      string
	Hostname         string
	ServiceAccount   string
	// Arbitrary key-value bag, populated from GCE instance metadata attributes.
	Attributes map[string]string
}

var defaultMeta = instanceMetadata{
	ProjectID:        "local-dev",
	NumericProjectID: "123456789012",
	InstanceName:     "minisky-local-vm",
	InstanceID:       "1234567890123456789",
	Zone:             "projects/local-dev/zones/us-central1-a",
	MachineType:      "projects/local-dev/machineTypes/n1-standard-1",
	Hostname:         "minisky-local-vm.us-central1-a.c.local-dev.internal",
	ServiceAccount:   "default@local-dev.iam.gserviceaccount.com",
	Attributes: map[string]string{
		"startup-script": "#!/bin/bash\necho 'MiniSky VM started'",
		"ssh-keys":       "user:ssh-rsa AAAA...localkey user@minisky",
	},
}

// API is a high-fidelity shim representing the GCP Metadata Server.
// It handles the well-known HTTP paths used by:
//   - google-auth-library (token fetching)
//   - gcloud SDK (project discovery)
//   - Terraform & Kubernetes (VM identity, zone, SA email)
type API struct {
	meta instanceMetadata
}

func NewAPI() *API {
	return &API{meta: defaultMeta}
}

// ServeHTTP implements http.Handler.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Metadata Server] %s %s", r.Method, r.URL.Path)

	// --- High-Fidelity Header Enforcement ---
	// The real metadata server requires this header on every request.
	// All GCP SDK clients set it automatically; missing it = 403.
	flavor := r.Header.Get("Metadata-Flavor")
	xGoogle := r.Header.Get("X-Google-Metadata-Request")
	if flavor != "Google" && xGoogle != "True" {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "Missing required header: Metadata-Flavor: Google")
		return
	}

	// Always identify ourselves like the real server does
	w.Header().Set("Metadata-Flavor", "Google")
	w.Header().Set("Server", "Metadata Server for VM")
	w.Header().Set("X-XSS-Protection", "0")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	path := r.URL.Path

	// Optional recursive dump — ?recursive=true returns full JSON tree
	if r.URL.Query().Get("recursive") == "true" {
		api.handleRecursive(w, r)
		return
	}

	switch {
	// ── PROJECT ─────────────────────────────────────────────────────────────
	case endsWith(path, "project/project-id"):
		plainText(w, api.meta.ProjectID)

	case endsWith(path, "project/numeric-project-id"):
		plainText(w, api.meta.NumericProjectID)

	case endsWith(path, "project/attributes/"):
		api.listAttributes(w, api.meta.Attributes)

	case strings.Contains(path, "project/attributes/"):
		key := lastSegment(path)
		api.getAttribute(w, api.meta.Attributes, key)

	// ── INSTANCE ────────────────────────────────────────────────────────────
	case endsWith(path, "instance/name"):
		plainText(w, api.meta.InstanceName)

	case endsWith(path, "instance/id"):
		plainText(w, api.meta.InstanceID)

	case endsWith(path, "instance/zone"):
		plainText(w, api.meta.Zone)

	case endsWith(path, "instance/machine-type"):
		plainText(w, api.meta.MachineType)

	case endsWith(path, "instance/hostname"):
		plainText(w, api.meta.Hostname)

	case endsWith(path, "instance/attributes/"):
		api.listAttributes(w, api.meta.Attributes)

	case strings.Contains(path, "instance/attributes/"):
		key := lastSegment(path)
		api.getAttribute(w, api.meta.Attributes, key)

	// ── SERVICE ACCOUNTS ─────────────────────────────────────────────────
	case endsWith(path, "instance/service-accounts/"):
		// List service accounts
		w.Header().Set("Content-Type", "application/text")
		w.WriteHeader(http.StatusOK)
		// Real GCS returns "default/\n<sa-email>/\n"
		fmt.Fprintf(w, "default/\n%s/\n", api.meta.ServiceAccount)

	case endsWith(path, "/email"):
		plainText(w, api.meta.ServiceAccount)

	case endsWith(path, "/aliases"):
		plainText(w, "default")

	case endsWith(path, "/scopes"):
		// Return the full list of OAuth2 scopes granted to the default SA
		scopes := strings.Join([]string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/devstorage.full_control",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/monitoring.write",
		}, "\n")
		plainText(w, scopes)

	case endsWith(path, "/token"):
		api.handleToken(w, r)

	case endsWith(path, "/identity"):
		// OIDC identity token — used by Cloud Run audience checks
		api.handleIdentityToken(w, r)

	// ── ROOT / UNKNOWN ─────────────────────────────────────────────────────
	case endsWith(path, "computeMetadata/v1/") || endsWith(path, "computeMetadata/v1"):
		api.handleRoot(w)

	default:
		log.Printf("[Shim: Metadata Server] Unhandled route: %s", path)
		w.WriteHeader(http.StatusNotFound)
	}
}

// handleToken serves a fake OAuth2 access token.
// GCP SDKs use this path to authenticate before every API call.
//
// Real path: /computeMetadata/v1/instance/service-accounts/default/token
// Real response: {"access_token":"...","expires_in":3599,"token_type":"Bearer"}
func (api *API) handleToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// The value itself is intentionally fake — it only needs to look like a token
	// so that GCP client libraries don't raise parsing errors.
	// Services like fake-gcs-server accept any valid-looking bearer without verifying it.
	fakeToken := map[string]interface{}{
		"access_token":  "minisky-dev-access-token-" + fmt.Sprintf("%d", time.Now().Unix()),
		"expires_in":    3599,
		"token_type":    "Bearer",
	}
	json.NewEncoder(w).Encode(fakeToken)
}

// handleIdentityToken serves a minimal JWT-shaped OIDC token.
// Used by Cloud Run audience checks, not cryptographically valid.
func (api *API) handleIdentityToken(w http.ResponseWriter, r *http.Request) {
	audience := r.URL.Query().Get("audience")
	if audience == "" {
		audience = "https://run.googleapis.com/"
	}
	// Return a fake (non-signed) JWT-like string. Client libs that only decode won't error.
	// Real enforcement must be disabled in the calling service for local dev.
	fakeJWT := "eyJhbGciOiJSUzI1NiJ9.minisky-identity-" + fmt.Sprintf("%d", time.Now().Unix())
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, fakeJWT)
}

// handleRoot returns the canonical directory listing for v1/ root.
func (api *API) handleRoot(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/text")
	w.WriteHeader(http.StatusOK)
	listing := "instance/\nproject/\n"
	fmt.Fprint(w, listing)
}

// handleRecursive dumps the full metadata tree as JSON (mirrors ?recursive=true behaviour).
func (api *API) handleRecursive(w http.ResponseWriter, r *http.Request) {
	tree := map[string]interface{}{
		"instance": map[string]interface{}{
			"name":        api.meta.InstanceName,
			"id":          api.meta.InstanceID,
			"zone":        api.meta.Zone,
			"machineType": api.meta.MachineType,
			"hostname":    api.meta.Hostname,
			"attributes":  api.meta.Attributes,
			"serviceAccounts": map[string]interface{}{
				"default": map[string]interface{}{
					"email":   api.meta.ServiceAccount,
					"aliases": []string{"default"},
					"scopes": []string{
						"https://www.googleapis.com/auth/cloud-platform",
						"https://www.googleapis.com/auth/devstorage.full_control",
						"https://www.googleapis.com/auth/logging.write",
						"https://www.googleapis.com/auth/monitoring.write",
					},
				},
			},
		},
		"project": map[string]interface{}{
			"projectId":        api.meta.ProjectID,
			"numericProjectId": api.meta.NumericProjectID,
			"attributes":       map[string]string{},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tree)
}

// listAttributes sends a plain-text directory listing of attribute keys.
func (api *API) listAttributes(w http.ResponseWriter, attrs map[string]string) {
	w.Header().Set("Content-Type", "application/text")
	w.WriteHeader(http.StatusOK)
	for k := range attrs {
		fmt.Fprintf(w, "%s\n", k)
	}
}

// getAttribute looks up one attribute key; 404 if missing.
func (api *API) getAttribute(w http.ResponseWriter, attrs map[string]string, key string) {
	val, ok := attrs[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	plainText(w, val)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func plainText(w http.ResponseWriter, value string) {
	w.Header().Set("Content-Type", "application/text")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, value)
}

func endsWith(path, suffix string) bool {
	return strings.HasSuffix(strings.TrimRight(path, "/"), strings.TrimRight(suffix, "/"))
}

func lastSegment(path string) string {
	path = strings.TrimRight(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
