package dns

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// ManagedZone mirrors the Cloud DNS ManagedZone resource.
type ManagedZone struct {
	Kind              string            `json:"kind"`
	Name              string            `json:"name"`
	DnsName           string            `json:"dnsName"`
	Description       string            `json:"description,omitempty"`
	ID                uint64            `json:"id,string"`
	NameServers       []string          `json:"nameServers"`
	CreationTime      string            `json:"creationTime"`
	Visibility        string            `json:"visibility"` // public, private
	DNSSECConfig      *DNSSECConfig     `json:"dnssecConfig,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	PrivateVisibilityConfig *PrivateVisibilityConfig `json:"privateVisibilityConfig,omitempty"`
}

type DNSSECConfig struct {
	State string `json:"state"` // off, on, transfer
}

type PrivateVisibilityConfig struct {
	Networks []PrivateNetwork `json:"networks"`
}

type PrivateNetwork struct {
	NetworkURL string `json:"networkUrl"`
	Kind       string `json:"kind"`
}

// ResourceRecordSet (RRSet) mirrors the Cloud DNS ResourceRecordSet.
// The key fields used by Terraform and gcloud are name, type, ttl, rrdatas.
type ResourceRecordSet struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`    // FQDN, e.g. "www.example.com."
	Type    string   `json:"type"`    // A, AAAA, CNAME, MX, TXT, NS, SOA, PTR, SRV, CAA
	TTL     int      `json:"ttl"`
	Rrdatas []string `json:"rrdatas"`
}

// Change represents an atomic batch of DNS record additions/deletions.
type Change struct {
	Kind        string               `json:"kind"`
	ID          string               `json:"id"`
	Status      string               `json:"status"` // pending → done
	StartTime   string               `json:"startTime"`
	Additions   []ResourceRecordSet  `json:"additions,omitempty"`
	Deletions   []ResourceRecordSet  `json:"deletions,omitempty"`
	IsServing   bool                 `json:"isServing"`
}

// ─────────────────────────────────────────────────────────────────────────────
// In-memory store helpers
// ─────────────────────────────────────────────────────────────────────────────

// zoneStore holds all data for a single managed zone.
type zoneStore struct {
	zone    *ManagedZone
	rrsets  map[string]*ResourceRecordSet // key: name+":"+type
	changes []*Change
	changeSeq int
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity Cloud DNS v1 shim.
type API struct {
	mu    sync.RWMutex
	zones map[string]*zoneStore // key: project:zoneName
	zoneSeq uint64
}

func NewAPI() *API {
	return &API{
		zones: make(map[string]*zoneStore),
	}
}

// ServeHTTP dispatches Cloud DNS v1 paths.
//
// Supported paths (dns.googleapis.com):
//   POST   /dns/v1/projects/{project}/managedZones
//   GET    /dns/v1/projects/{project}/managedZones
//   GET    /dns/v1/projects/{project}/managedZones/{zone}
//   PATCH  /dns/v1/projects/{project}/managedZones/{zone}
//   DELETE /dns/v1/projects/{project}/managedZones/{zone}
//   GET    /dns/v1/projects/{project}/managedZones/{zone}/rrsets
//   POST   /dns/v1/projects/{project}/managedZones/{zone}/rrsets
//   DELETE /dns/v1/projects/{project}/managedZones/{zone}/rrsets/{name}/{type}
//   POST   /dns/v1/projects/{project}/managedZones/{zone}/changes
//   GET    /dns/v1/projects/{project}/managedZones/{zone}/changes
//   GET    /dns/v1/projects/{project}/managedZones/{zone}/changes/{changeId}
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Cloud DNS] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	project := extractSegmentAfter(path, "projects")

	switch {
	case strings.Contains(path, "/changes"):
		zoneName := extractSegmentAfter(path, "managedZones")
		api.routeChanges(w, r, project, zoneName, path)

	case strings.Contains(path, "/rrsets"):
		zoneName := extractSegmentAfter(path, "managedZones")
		api.routeRRSets(w, r, project, zoneName, path)

	case strings.Contains(path, "/managedZones"):
		api.routeZones(w, r, project, path)

	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Cloud DNS resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Managed Zones
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeZones(w http.ResponseWriter, r *http.Request, project, path string) {
	zoneName := extractSegmentAfter(path, "managedZones")

	switch r.Method {
	case http.MethodPost:
		api.createZone(w, r, project)
	case http.MethodGet:
		if zoneName != "" {
			api.getZone(w, project, zoneName)
		} else {
			api.listZones(w, r, project)
		}
	case http.MethodPatch:
		api.patchZone(w, r, project, zoneName)
	case http.MethodDelete:
		api.deleteZone(w, project, zoneName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createZone(w http.ResponseWriter, r *http.Request, project string) {
	var body ManagedZone
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if body.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "'name' is required")
		return
	}
	if body.DnsName == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "'dnsName' is required")
		return
	}

	// Ensure dnsName is dot-terminated (FQDN)
	dnsName := body.DnsName
	if !strings.HasSuffix(dnsName, ".") {
		dnsName += "."
	}

	visibility := body.Visibility
	if visibility == "" {
		visibility = "public"
	}

	api.mu.Lock()
	api.zoneSeq++
	id := api.zoneSeq
	api.mu.Unlock()

	zone := &ManagedZone{
		Kind:         "dns#managedZone",
		Name:         body.Name,
		DnsName:      dnsName,
		Description:  body.Description,
		ID:           id,
		Labels:       body.Labels,
		Visibility:   visibility,
		DNSSECConfig: body.DNSSECConfig,
		PrivateVisibilityConfig: body.PrivateVisibilityConfig,
		CreationTime: time.Now().UTC().Format(time.RFC3339),
		// Return realistic-looking MiniSky name servers
		NameServers: []string{
			fmt.Sprintf("ns-cloud-a1.minisky.dev."),
			fmt.Sprintf("ns-cloud-a2.minisky.dev."),
			fmt.Sprintf("ns-cloud-a3.minisky.dev."),
			fmt.Sprintf("ns-cloud-a4.minisky.dev."),
		},
	}

	// Seed the zone with mandatory SOA and NS records (mirrors GCP behaviour)
	soaRdata := fmt.Sprintf("ns-cloud-a1.minisky.dev. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300")
	nsRdatas := zone.NameServers

	store := &zoneStore{
		zone:   zone,
		rrsets: map[string]*ResourceRecordSet{
			rrKey(dnsName, "SOA"): {
				Kind:    "dns#resourceRecordSet",
				Name:    dnsName,
				Type:    "SOA",
				TTL:     21600,
				Rrdatas: []string{soaRdata},
			},
			rrKey(dnsName, "NS"): {
				Kind:    "dns#resourceRecordSet",
				Name:    dnsName,
				Type:    "NS",
				TTL:     21600,
				Rrdatas: nsRdatas,
			},
		},
	}

	key := zoneKey(project, body.Name)
	api.mu.Lock()
	api.zones[key] = store
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(zone)
}

func (api *API) getZone(w http.ResponseWriter, project, zoneName string) {
	key := zoneKey(project, zoneName)
	api.mu.RLock()
	store, ok := api.zones[key]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("ManagedZone '%s' not found in project '%s'", zoneName, project))
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(store.zone)
}

func (api *API) listZones(w http.ResponseWriter, r *http.Request, project string) {
	// Optional ?dnsName= filter
	filterDNS := r.URL.Query().Get("dnsName")

	prefix := project + ":"
	api.mu.RLock()
	items := []*ManagedZone{}
	for k, v := range api.zones {
		if strings.HasPrefix(k, prefix) {
			if filterDNS == "" || strings.EqualFold(v.zone.DnsName, filterDNS) {
				items = append(items, v.zone)
			}
		}
	}
	api.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":         "dns#managedZonesListResponse",
		"managedZones": items,
	})
}

func (api *API) patchZone(w http.ResponseWriter, r *http.Request, project, zoneName string) {
	key := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[key]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	var patch ManagedZone
	json.NewDecoder(r.Body).Decode(&patch)
	if patch.Description != "" {
		store.zone.Description = patch.Description
	}
	if patch.Labels != nil {
		store.zone.Labels = patch.Labels
	}
	zone := store.zone
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(zone)
}

func (api *API) deleteZone(w http.ResponseWriter, project, zoneName string) {
	key := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[key]
	if ok {
		// GCP refuses to delete a zone that has non-SOA/NS records
		nonSystem := 0
		for _, rr := range store.rrsets {
			if rr.Type != "SOA" && rr.Type != "NS" {
				nonSystem++
			}
		}
		if nonSystem > 0 {
			api.mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "FAILED_PRECONDITION",
				fmt.Sprintf("Zone '%s' cannot be deleted because it still contains non-NS/SOA resource record sets", zoneName))
			return
		}
		delete(api.zones, key)
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}
	// GCP returns 204 No Content on delete
	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource Record Sets
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeRRSets(w http.ResponseWriter, r *http.Request, project, zoneName, path string) {
	// Path for DELETE: /rrsets/{name}/{type}
	// We detect this by checking segments after "rrsets"
	rrName, rrType := extractRRPath(path)

	switch r.Method {
	case http.MethodPost:
		api.createRRSet(w, r, project, zoneName)
	case http.MethodGet:
		api.listRRSets(w, r, project, zoneName)
	case http.MethodDelete:
		if rrName == "" || rrType == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "Resource record set name and type are required for delete")
			return
		}
		api.deleteRRSet(w, project, zoneName, rrName, rrType)
	case http.MethodPut:
		// PATCH/PUT updates a single RRSet
		api.putRRSet(w, r, project, zoneName, rrName, rrType)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createRRSet(w http.ResponseWriter, r *http.Request, project, zoneName string) {
	key := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[key]
	api.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	var rr ResourceRecordSet
	if err := json.NewDecoder(r.Body).Decode(&rr); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if rr.Name == "" || rr.Type == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "'name' and 'type' are required")
		return
	}
	// Ensure FQDN
	if !strings.HasSuffix(rr.Name, ".") {
		rr.Name += "."
	}
	if rr.TTL == 0 {
		rr.TTL = 300
	}
	rr.Kind = "dns#resourceRecordSet"

	rrk := rrKey(rr.Name, rr.Type)
	api.mu.Lock()
	// Check for duplicate
	if _, exists := store.rrsets[rrk]; exists {
		api.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		writeError(w, 409, "ALREADY_EXISTS",
			fmt.Sprintf("ResourceRecordSet '%s' of type '%s' already exists", rr.Name, rr.Type))
		return
	}
	store.rrsets[rrk] = &rr
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&rr)
}

func (api *API) listRRSets(w http.ResponseWriter, r *http.Request, project, zoneName string) {
	key := zoneKey(project, zoneName)
	api.mu.RLock()
	store, ok := api.zones[key]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	// Optional filters: ?name=, ?type=
	filterName := r.URL.Query().Get("name")
	filterType := r.URL.Query().Get("type")

	api.mu.RLock()
	items := []*ResourceRecordSet{}
	for _, v := range store.rrsets {
		if filterName != "" && v.Name != filterName {
			continue
		}
		if filterType != "" && v.Type != filterType {
			continue
		}
		items = append(items, v)
	}
	api.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":               "dns#resourceRecordSetsListResponse",
		"rrsets":             items,
		"nextPageToken":      "",
	})
}

func (api *API) deleteRRSet(w http.ResponseWriter, project, zoneName, name, rrType string) {
	// Ensure FQDN
	if !strings.HasSuffix(name, ".") {
		name += "."
	}

	key := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[key]
	if ok {
		rrk := rrKey(name, strings.ToUpper(rrType))
		if _, exists := store.rrsets[rrk]; !exists {
			ok = false
		} else {
			delete(store.rrsets, rrk)
		}
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("ResourceRecordSet '%s/%s' not found in zone '%s'", name, rrType, zoneName))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (api *API) putRRSet(w http.ResponseWriter, r *http.Request, project, zoneName, name, rrType string) {
	key := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[key]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	var rr ResourceRecordSet
	json.NewDecoder(r.Body).Decode(&rr)
	if !strings.HasSuffix(rr.Name, ".") {
		rr.Name += "."
	}
	rr.Kind = "dns#resourceRecordSet"
	store.rrsets[rrKey(rr.Name, rr.Type)] = &rr
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&rr)
}

// ─────────────────────────────────────────────────────────────────────────────
// Changes  (atomic batches of additions + deletions)
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeChanges(w http.ResponseWriter, r *http.Request, project, zoneName, path string) {
	changeID := extractSegmentAfter(path, "changes")

	switch r.Method {
	case http.MethodPost:
		api.createChange(w, r, project, zoneName)
	case http.MethodGet:
		if changeID != "" {
			api.getChange(w, project, zoneName, changeID)
		} else {
			api.listChanges(w, project, zoneName)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// createChange applies an atomic set of DNS additions and deletions.
// The GCP spec says: deletions are applied before additions in the same request.
func (api *API) createChange(w http.ResponseWriter, r *http.Request, project, zoneName string) {
	zKey := zoneKey(project, zoneName)
	api.mu.Lock()
	store, ok := api.zones[zKey]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	var body Change
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	// Validate: deletions must exist
	for _, del := range body.Deletions {
		name := del.Name
		if !strings.HasSuffix(name, ".") {
			name += "."
		}
		rrk := rrKey(name, del.Type)
		if _, exists := store.rrsets[rrk]; !exists {
			api.mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND",
				fmt.Sprintf("Cannot delete non-existent ResourceRecordSet '%s' of type '%s'", del.Name, del.Type))
			return
		}
	}

	// Apply deletions first
	for _, del := range body.Deletions {
		name := del.Name
		if !strings.HasSuffix(name, ".") {
			name += "."
		}
		delete(store.rrsets, rrKey(name, del.Type))
	}

	// Apply additions
	for i, add := range body.Additions {
		name := add.Name
		if !strings.HasSuffix(name, ".") {
			name += "."
		}
		if add.TTL == 0 {
			add.TTL = 300
		}
		body.Additions[i].Kind = "dns#resourceRecordSet"
		body.Additions[i].Name = name
		store.rrsets[rrKey(name, add.Type)] = &body.Additions[i]
	}

	// Record the change
	store.changeSeq++
	changeID := fmt.Sprintf("%d", store.changeSeq)
	change := &Change{
		Kind:      "dns#change",
		ID:        changeID,
		Status:    "done", // Cloud DNS changes are synchronous; status flips to done immediately
		StartTime: time.Now().UTC().Format(time.RFC3339),
		Additions: body.Additions,
		Deletions: body.Deletions,
		IsServing: true,
	}
	store.changes = append(store.changes, change)
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(change)
}

func (api *API) getChange(w http.ResponseWriter, project, zoneName, changeID string) {
	zKey := zoneKey(project, zoneName)
	api.mu.RLock()
	store, ok := api.zones[zKey]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	api.mu.RLock()
	var found *Change
	for _, c := range store.changes {
		if c.ID == changeID {
			found = c
			break
		}
	}
	api.mu.RUnlock()

	if found == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Change "+changeID+" not found in zone "+zoneName)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(found)
}

func (api *API) listChanges(w http.ResponseWriter, project, zoneName string) {
	zKey := zoneKey(project, zoneName)
	api.mu.RLock()
	store, ok := api.zones[zKey]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "ManagedZone "+zoneName+" not found")
		return
	}

	api.mu.RLock()
	changes := make([]*Change, len(store.changes))
	copy(changes, store.changes)
	api.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":    "dns#changesListResponse",
		"changes": changes,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func zoneKey(project, zoneName string) string { return project + ":" + zoneName }
func rrKey(name, rrType string) string        { return name + ":" + strings.ToUpper(rrType) }

// extractSegmentAfter returns the path segment immediately after keyword.
func extractSegmentAfter(path, keyword string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == keyword && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractRRPath returns (name, type) for DELETE /rrsets/{name}/{type}.
// Cloud DNS encodes the name as a URL segment, but it may contain dots.
func extractRRPath(path string) (string, string) {
	idx := strings.Index(path, "/rrsets/")
	if idx == -1 {
		return "", ""
	}
	rest := path[idx+len("/rrsets/"):]
	// rest = "{name}/{type}" where name is URL-path-encoded FQDN
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash == -1 {
		return "", ""
	}
	return rest[:lastSlash], rest[lastSlash+1:]
}

func writeError(w http.ResponseWriter, code int, status, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"status":  status,
			"message": message,
		},
	})
}
