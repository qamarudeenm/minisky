package compute

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"minisky/pkg/orchestrator"
)

// TestMain points HOME at a temporary directory for the whole package. The
// shim now loads and saves state under ~/.minisky during construction, and a
// test that read the developer's real state would both fail unpredictably and
// overwrite it.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "minisky-compute-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func request(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
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

// isolate gives one test its own state file, so restarts in one test are not
// visible to another.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// A network read as absent after a restart is a network Terraform plans to
// create again, and the create then fails on the VPC that is still there.
func TestNetworkSurvivesARestart(t *testing.T) {
	isolate(t)
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, "{}")
	}}
	svcMgr := orchestrator.NewServiceManagerForTesting(ft)

	api := NewAPI(orchestrator.NewOperationManager(), svcMgr)
	code, _ := request(t, api, http.MethodPost, "/compute/v1/projects/probe/global/networks",
		`{"name":"analytics","description":"pipeline VPC","autoCreateSubnetworks":false}`)
	if code != http.StatusOK {
		t.Fatalf("create network = %d, want 200", code)
	}

	restarted := NewAPI(orchestrator.NewOperationManager(), svcMgr)
	code, n := request(t, restarted, http.MethodGet,
		"/compute/v1/projects/probe/global/networks/analytics", "")
	if code != http.StatusOK {
		t.Fatalf("GET network after restart = %d, want 200", code)
	}
	if n["description"] != "pipeline VPC" {
		t.Errorf("description = %v after restart", n["description"])
	}
}

// A forgotten firewall rule is not merely invisible: the service manager's
// enforcement registry is rebuilt from these, so the rule silently stops being
// applied while it is still in the user's Terraform state.
func TestFirewallRuleIsReregisteredAfterARestart(t *testing.T) {
	isolate(t)
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"NetworkSettings":{"Networks":{"minisky-net":{}},"Ports":{}}}`)
	}}

	api := NewAPI(orchestrator.NewOperationManager(), orchestrator.NewServiceManagerForTesting(ft))
	code, _ := request(t, api, http.MethodPost, "/compute/v1/projects/probe/global/firewalls",
		`{"name":"allow-http","network":"https://www.googleapis.com/compute/v1/projects/probe/global/networks/analytics",
		  "direction":"INGRESS","sourceRanges":["0.0.0.0/0"],
		  "allowed":[{"IPProtocol":"tcp","ports":["80"]}]}`)
	if code != http.StatusOK {
		t.Fatalf("create firewall = %d, want 200", code)
	}

	// A fresh service manager is what a restart gives the shim: its firewall
	// registry starts empty and has to be rebuilt from the restored rules.
	restartedMgr := orchestrator.NewServiceManagerForTesting(ft)
	restarted := NewAPI(orchestrator.NewOperationManager(), restartedMgr)

	if code, _ := request(t, restarted, http.MethodGet,
		"/compute/v1/projects/probe/global/firewalls/allow-http", ""); code != http.StatusOK {
		t.Fatalf("GET firewall after restart = %d, want 200", code)
	}
	if !restartedMgr.CheckFirewallAllows("analytics", "tcp", "80", "10.0.0.1") {
		t.Error("the restored rule is not being enforced; it was not re-registered with the service manager")
	}
}

// The stored status is the shim's last word, not a reading of the world. A
// container removed while the daemon was down must not come back as an
// instance the caller can act on.
func TestInstanceStatusIsReconciledAgainstDocker(t *testing.T) {
	isolate(t)

	present := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/json") {
			return jsonResp(http.StatusOK, `{"State":{"Status":"exited"},"NetworkSettings":{"Networks":{}},"Ports":{}}`)
		}
		return jsonResp(http.StatusOK, "{}")
	}}

	api := NewAPI(orchestrator.NewOperationManager(), orchestrator.NewServiceManagerForTesting(present))
	inst := &Instance{Name: "worker-1", Status: "RUNNING"}
	inst.setContainerMapping("minisky-vm-worker-1")
	api.mu.Lock()
	api.instances[instanceKey("probe", "us-central1-a", "worker-1")] = inst
	api.saveLocked()
	api.mu.Unlock()

	// Docker reports the container stopped.
	restarted := NewAPI(orchestrator.NewOperationManager(), orchestrator.NewServiceManagerForTesting(present))
	code, got := request(t, restarted, http.MethodGet,
		"/compute/v1/projects/probe/zones/us-central1-a/instances/worker-1", "")
	if code != http.StatusOK {
		t.Fatalf("GET instance after restart = %d, want 200", code)
	}
	if got["status"] != "TERMINATED" {
		t.Errorf("status = %v, want TERMINATED — the container is not running", got["status"])
	}

	// Docker no longer has the container at all.
	gone := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusNotFound, `{"message":"No such container"}`)
	}}
	afterRemoval := NewAPI(orchestrator.NewOperationManager(), orchestrator.NewServiceManagerForTesting(gone))
	if code, _ := request(t, afterRemoval, http.MethodGet,
		"/compute/v1/projects/probe/zones/us-central1-a/instances/worker-1", ""); code != http.StatusNotFound {
		t.Errorf("GET a removed instance = %d, want 404", code)
	}
}
