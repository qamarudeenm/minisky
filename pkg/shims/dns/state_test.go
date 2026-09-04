package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func call(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	var decoded map[string]interface{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec.Code, decoded
}

// A zone's numeric id is part of its identity, so a restart that regenerated it
// would make the same zone read back as a different resource.
func TestManagedZoneAndRecordsSurviveARestart(t *testing.T) {
	isolate(t)

	const zones = "/dns/v1/projects/probe/managedZones"
	api := NewAPI()
	code, created := call(t, api, http.MethodPost, zones,
		`{"name":"internal","dnsName":"internal.example.com.","description":"service discovery"}`)
	if code != http.StatusOK {
		t.Fatalf("create zone = %d, want 200", code)
	}
	id := created["id"]
	if id == nil {
		t.Fatalf("no id on the created zone: %v", created)
	}

	if code, _ := call(t, api, http.MethodPost, zones+"/internal/rrsets",
		`{"name":"api.internal.example.com.","type":"A","ttl":300,"rrdatas":["10.0.0.5"]}`); code != http.StatusOK {
		t.Fatalf("create rrset = %d, want 200", code)
	}

	restarted := NewAPI()
	code, zone := call(t, restarted, http.MethodGet, zones+"/internal", "")
	if code != http.StatusOK {
		t.Fatalf("GET zone after restart = %d, want 200", code)
	}
	if zone["id"] != id {
		t.Errorf("id = %v after restart, want %v — the zone reads as a different resource", zone["id"], id)
	}
	if zone["description"] != "service discovery" {
		t.Errorf("description = %v after restart", zone["description"])
	}

	code, rrsets := call(t, restarted, http.MethodGet, zones+"/internal/rrsets", "")
	if code != http.StatusOK {
		t.Fatalf("list rrsets after restart = %d, want 200", code)
	}
	items, _ := rrsets["rrsets"].([]interface{})
	found := false
	for _, item := range items {
		if rr, ok := item.(map[string]interface{}); ok && rr["name"] == "api.internal.example.com." {
			found = true
		}
	}
	if !found {
		t.Errorf("the A record did not survive the restart: %v", rrsets["rrsets"])
	}
}

// Ids come from a counter. If it restarted at zero, a new zone would be handed
// an id a previous one already used.
func TestZoneIDsDoNotRepeatAfterARestart(t *testing.T) {
	isolate(t)

	const zones = "/dns/v1/projects/probe/managedZones"
	api := NewAPI()
	_, first := call(t, api, http.MethodPost, zones, `{"name":"one","dnsName":"one.example.com."}`)

	restarted := NewAPI()
	_, second := call(t, restarted, http.MethodPost, zones, `{"name":"two","dnsName":"two.example.com."}`)

	if first["id"] == second["id"] {
		t.Errorf("both zones were given id %v; the counter restarted", first["id"])
	}
}
