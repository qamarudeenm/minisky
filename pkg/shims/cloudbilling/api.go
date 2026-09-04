package cloudbilling

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"minisky/pkg/registry"
)

// ─────────────────────────────────────────────────────────────────────────────
// Cloud Billing
//
// This exists because google_project cannot be read without it. Terraform reads
// a project's billing account as part of every refresh, so without an answer
// here that call escapes to the real cloudbilling.googleapis.com and fails
// authentication — the project is created, then immediately unusable.
//
// The surface is deliberately small: the billing account a project is linked
// to, and the accounts available to link. Nothing is charged, and no cost is
// modelled.
// ─────────────────────────────────────────────────────────────────────────────

func init() {
	registry.Register("cloudbilling.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI()
	})

	registry.RegisterRoutes("cloudbilling.googleapis.com",
		"/v1/projects/*/billingInfo",
		"/v1/billingAccounts",
	)
}

// defaultAccount is the account MiniSky offers, so a configuration that sets
// billing_account has something real to point at.
const defaultAccount = "billingAccounts/01A2B3-C4D5E6-F7G8H9"

type ProjectBillingInfo struct {
	Name               string `json:"name"`
	ProjectID          string `json:"projectId"`
	BillingAccountName string `json:"billingAccountName"`
	BillingEnabled     bool   `json:"billingEnabled"`
}

type BillingAccount struct {
	Name        string `json:"name"`
	Open        bool   `json:"open"`
	DisplayName string `json:"displayName"`
}

type API struct {
	mu       sync.RWMutex
	linked   map[string]string // projectId -> billingAccounts/{id}
	accounts map[string]*BillingAccount
}

func NewAPI() *API {
	api := &API{
		linked: map[string]string{},
		accounts: map[string]*BillingAccount{
			defaultAccount: {Name: defaultAccount, Open: true, DisplayName: "MiniSky Local Billing"},
		},
	}
	api.restore()
	return api
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer api.persistIfMutated(r)

	path := strings.TrimSuffix(r.URL.Path, "/")
	log.Printf("[Shim: CloudBilling] %s %s", r.Method, path)
	w.Header().Set("Content-Type", "application/json")

	switch {
	case strings.HasSuffix(path, "/billingInfo"):
		api.routeBillingInfo(w, r, path)
	case strings.HasPrefix(path, "/v1/billingAccounts"):
		api.routeAccounts(w, r, path)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown Cloud Billing path "+path)
	}
}

func (api *API) routeBillingInfo(w http.ResponseWriter, r *http.Request, path string) {
	projectID := segmentAfter(path, "projects")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "a project is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		api.mu.RLock()
		account := api.linked[projectID]
		api.mu.RUnlock()
		writeJSON(w, billingInfo(projectID, account))

	case http.MethodPut, http.MethodPatch:
		var body ProjectBillingInfo
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
			return
		}

		api.mu.Lock()
		if body.BillingAccountName == "" {
			delete(api.linked, projectID) // unlinking disables billing
		} else {
			api.linked[projectID] = body.BillingAccountName
		}
		account := api.linked[projectID]
		api.mu.Unlock()

		writeJSON(w, billingInfo(projectID, account))

	default:
		writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
			r.Method+" is not supported on "+path)
	}
}

func (api *API) routeAccounts(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
			"billing accounts are provided by MiniSky and cannot be created")
		return
	}

	api.mu.RLock()
	defer api.mu.RUnlock()

	if id := segmentAfter(path, "billingAccounts"); id != "" {
		account, ok := api.accounts["billingAccounts/"+id]
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "billingAccounts/"+id+" not found")
			return
		}
		writeJSON(w, account)
		return
	}

	accounts := []*BillingAccount{}
	for _, a := range api.accounts {
		accounts = append(accounts, a)
	}
	writeJSON(w, map[string]interface{}{"billingAccounts": accounts})
}

// billingInfo renders the resource. billingEnabled follows from whether an
// account is linked, exactly as it does in Google Cloud.
func billingInfo(projectID, account string) *ProjectBillingInfo {
	return &ProjectBillingInfo{
		Name:               "projects/" + projectID + "/billingInfo",
		ProjectID:          projectID,
		BillingAccountName: account,
		BillingEnabled:     account != "",
	}
}

func segmentAfter(path, segment string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == segment && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, body interface{}) {
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, status, message string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"code": code, "message": message, "status": status},
	})
}
