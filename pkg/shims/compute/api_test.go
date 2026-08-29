package compute

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minisky/pkg/orchestrator"
)

// fakeTransport routes requests to a handler so tests can fake the Docker
// daemon's HTTP API without a real socket.
type fakeTransport struct {
	handler func(req *http.Request) (*http.Response, error)
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.handler(req)
}

func jsonResp(status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestExtractNameFromURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://www.googleapis.com/compute/v1/projects/p/global/networks/shared", "shared"},
		{"default", "default"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractNameFromURL(tt.in); got != tt.want {
			t.Errorf("extractNameFromURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDockerNetworkNameForVPC(t *testing.T) {
	tests := []struct {
		vpcName string
		want    string
	}{
		{"", "minisky-net"},
		{"default", "minisky-net"},
		{"shared", "minisky-vpc-shared"},
	}
	for _, tt := range tests {
		if got := dockerNetworkNameForVPC(tt.vpcName); got != tt.want {
			t.Errorf("dockerNetworkNameForVPC(%q) = %q, want %q", tt.vpcName, got, tt.want)
		}
	}
}

func TestGetAllowedPortsForVPC(t *testing.T) {
	api := NewAPI(nil, nil)
	api.firewalls["p:allow-default"] = &FirewallRule{
		Network:   "https://www.googleapis.com/compute/v1/projects/p/global/networks/default",
		Direction: "INGRESS",
		Action:    "allow",
		Allowed:   []FirewallAllow{{Ports: []string{"22"}}},
	}
	api.firewalls["p:allow-shared"] = &FirewallRule{
		Network:   "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared",
		Direction: "INGRESS",
		Action:    "allow",
		Allowed:   []FirewallAllow{{Ports: []string{"80", "443"}}},
	}
	api.firewalls["p:deny-shared"] = &FirewallRule{
		Network:   "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared",
		Direction: "INGRESS",
		Action:    "deny",
		Allowed:   []FirewallAllow{{Ports: []string{"3389"}}},
	}

	if got := api.getAllowedPortsForVPC("default"); !equalSet(got, []string{"22"}) {
		t.Errorf("default ports = %v, want [22]", got)
	}
	if got := api.getAllowedPortsForVPC("shared"); !equalSet(got, []string{"80", "443"}) {
		t.Errorf("shared ports = %v, want [80 443]", got)
	}
}

func equalSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestPatchNetworkIPs_PerInterfaceOnItsOwnNetwork(t *testing.T) {
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"NetworkSettings":{"Networks":{
			"minisky-net": {"IPAddress": "172.18.0.2"},
			"minisky-vpc-shared": {"IPAddress": "172.19.0.5"}
		}}}`)
	}}
	svcMgr := orchestrator.NewServiceManagerForTesting(ft)
	api := NewAPI(nil, svcMgr)

	inst := &Instance{
		NetworkInterfaces: []NetworkInterface{
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared"},
		},
	}

	api.patchNetworkIPs(inst, "my-vm")

	if inst.NetworkInterfaces[0].NetworkIP != "172.18.0.2" {
		t.Errorf("nic0 IP = %q, want 172.18.0.2", inst.NetworkInterfaces[0].NetworkIP)
	}
	if inst.NetworkInterfaces[1].NetworkIP != "172.19.0.5" {
		t.Errorf("nic1 IP = %q, want 172.19.0.5", inst.NetworkInterfaces[1].NetworkIP)
	}
}

func TestReapplyFirewallToVPC_MatchesSecondaryNIC(t *testing.T) {
	var calls []string
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		switch {
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/json"):
			return jsonResp(http.StatusOK, `{"NetworkSettings":{"Networks":{"minisky-net":{}},"Ports":{}}}`)
		default:
			return jsonResp(http.StatusOK, "{}")
		}
	}}
	svcMgr := orchestrator.NewServiceManagerForTesting(ft)
	api := NewAPI(nil, svcMgr)

	// nic0 is "default", nic1 is "prod" — a firewall change on "prod" must
	// still pick this instance up even though it's not the primary NIC.
	api.instances["p:z:vm1"] = &Instance{
		Name: "vm1",
		NetworkInterfaces: []NetworkInterface{
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/prod"},
		},
	}

	api.reapplyFirewallToVPC("https://www.googleapis.com/compute/v1/projects/p/global/networks/prod")

	var sawDelete bool
	for _, c := range calls {
		if strings.HasPrefix(c, "DELETE") {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Errorf("expected reapplyFirewallToVPC to recreate the instance whose secondary NIC is on the changed VPC, calls were: %v", calls)
	}
}

func TestPatchNetworkIPs_FallsBackWhenNetworkMissing(t *testing.T) {
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"NetworkSettings":{"Networks":{}}}`)
	}}
	svcMgr := orchestrator.NewServiceManagerForTesting(ft)
	api := NewAPI(nil, svcMgr)

	inst := &Instance{
		NetworkInterfaces: []NetworkInterface{
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
		},
	}

	api.patchNetworkIPs(inst, "my-vm")

	if inst.NetworkInterfaces[0].NetworkIP != "10.128.0.2" {
		t.Errorf("nic0 IP = %q, want fallback 10.128.0.2", inst.NetworkInterfaces[0].NetworkIP)
	}
}

func TestGetInstance_SubnetworkBackfilledForEveryInterface(t *testing.T) {
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"NetworkSettings":{"Networks":{
			"minisky-net": {"IPAddress": "172.18.0.2"},
			"minisky-vpc-shared": {"IPAddress": "172.19.0.5"}
		}}}`)
	}}
	svcMgr := orchestrator.NewServiceManagerForTesting(ft)
	api := NewAPI(nil, svcMgr)

	// nic1 (on "shared") has no explicit Subnetwork, same as a client that never set
	// one — getInstance should backfill it just like it already does for nic0.
	api.instances[instanceKey("p", "us-central1-a", "vm1")] = &Instance{
		Name: "vm1",
		NetworkInterfaces: []NetworkInterface{
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
			{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared"},
		},
	}

	w := httptest.NewRecorder()
	api.getInstance(w, httptest.NewRequest(http.MethodGet, "/", nil), "p", "us-central1-a", "vm1")

	var got Instance
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for j, iface := range got.NetworkInterfaces {
		if iface.Subnetwork == "" {
			t.Errorf("nic%d Subnetwork is empty, want it backfilled like nic0 already was", j)
		}
	}
}

func TestDuplicateNetworkInterfaceVPC(t *testing.T) {
	tests := []struct {
		name   string
		ifaces []NetworkInterface
		want   string
	}{
		{
			name: "distinct networks, no duplicate",
			ifaces: []NetworkInterface{
				{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
				{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared"},
			},
			want: "",
		},
		{
			name: "same network on two NICs is a duplicate",
			ifaces: []NetworkInterface{
				{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared"},
				{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared"},
			},
			want: "shared",
		},
		{
			name: "empty network field normalizes to default and collides with an explicit default",
			ifaces: []NetworkInterface{
				{Network: "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
				{Network: ""},
			},
			want: "default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := duplicateNetworkInterfaceVPC(tt.ifaces); got != tt.want {
				t.Errorf("duplicateNetworkInterfaceVPC() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInsertInstance_RejectsDuplicateNetworkInterfaceVPC(t *testing.T) {
	// Real GCE rejects instances.insert outright when two network interfaces
	// reference the same VPC network — it never silently attaches only one.
	api := NewAPI(orchestrator.NewOperationManager(), orchestrator.NewServiceManagerForTesting(&fakeTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(http.StatusOK, "{}")
		},
	}))

	body := `{
		"name": "vm1",
		"networkInterfaces": [
			{"network": "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"},
			{"network": "https://www.googleapis.com/compute/v1/projects/p/global/networks/default"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/compute/v1/projects/p/zones/us-central1-a/instances", strings.NewReader(body))
	w := httptest.NewRecorder()
	api.insertInstance(w, req, "p", "us-central1-a")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
	if _, ok := api.instances[instanceKey("p", "us-central1-a", "vm1")]; ok {
		t.Error("instance should not have been created when networkInterfaces are invalid")
	}
}

func TestCreateFirewall_RegistersRuleUnderShortVPCName(t *testing.T) {
	// createFirewall stores the rule keyed by the full network URL in
	// api.firewalls (fine, getAllowedPortsForVPC extracts the short name at read
	// time there), but it must register with the orchestrator under the short VPC
	// name — allowedPortsForVPC/ApplyFirewallPortsToVPC always look it up by short
	// name, so registering under the full URL would make that lookup permanently
	// miss and silently close every port on a firewall-triggered VM recreate.
	svcMgr := orchestrator.NewServiceManagerForTesting(&fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, "{}")
	}})
	api := NewAPI(orchestrator.NewOperationManager(), svcMgr)

	body := `{
		"name": "allow-web",
		"network": "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared",
		"direction": "INGRESS",
		"action": "allow",
		"allowed": [{"IPProtocol": "tcp", "ports": ["8080"]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/compute/v1/projects/p/global/firewalls", strings.NewReader(body))
	w := httptest.NewRecorder()
	api.createFirewall(w, req, "p")

	if !svcMgr.CheckFirewallAllows("shared", "tcp", "8080", "1.2.3.4") {
		t.Error("firewall rule registered by createFirewall is not visible under the short VPC name 'shared'")
	}
}

func TestResolveOsImage(t *testing.T) {
	tests := map[string]string{
		"projects/ubuntu-os-cloud/global/images/family/ubuntu-2404-lts": "ubuntu:24.04",
		"ubuntu-2404-lts": "ubuntu:24.04",
		"ubuntu-2204-lts": "ubuntu:22.04",
		"debian-12":       "debian:12",
		"ubuntu:24.04":    "ubuntu:24.04", // raw Docker references pass through
		"":                "",
		"no-such-family":  "",
	}
	for in, want := range tests {
		if got := resolveOsImage(in); got != want {
			t.Errorf("resolveOsImage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInsertInstanceUsesInitializeParamsImage(t *testing.T) {
	// boot_disk.initialize_params.image is how Terraform asks for an OS; before
	// initializeParams was modelled the request decoded to an empty disk and
	// every VM silently booted the default image.
	var body struct {
		Disks []AttachedDisk `json:"disks"`
	}
	raw := `{"disks":[{"boot":true,"autoDelete":true,"initializeParams":{"sourceImage":"ubuntu-2204-lts","diskSizeGb":"50","diskType":"pd-balanced"}}]}`
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(body.Disks) != 1 || body.Disks[0].InitializeParams == nil {
		t.Fatalf("initializeParams not decoded: %+v", body.Disks)
	}
	if got := resolveOsImage(body.Disks[0].InitializeParams.SourceImage); got != "ubuntu:22.04" {
		t.Errorf("resolved image = %q, want ubuntu:22.04", got)
	}
}

// GCE reports networkFirewallPolicyEnforcementOrder on every network. While the
// shim omitted it, terraform proposed the same in-place update on every plan and
// the configuration never converged.
func TestNetworkReportsFirewallPolicyEnforcementOrder(t *testing.T) {
	var network Network
	raw := `{"kind":"compute#network","name":"vpc","networkFirewallPolicyEnforcementOrder":"BEFORE_CLASSIC_FIREWALL"}`
	if err := json.Unmarshal([]byte(raw), &network); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if network.NetworkFirewallPolicyEnforcementOrder != "BEFORE_CLASSIC_FIREWALL" {
		t.Errorf("enforcement order = %q, want BEFORE_CLASSIC_FIREWALL",
			network.NetworkFirewallPolicyEnforcementOrder)
	}

	encoded, err := json.Marshal(Network{
		Name: "vpc", NetworkFirewallPolicyEnforcementOrder: "AFTER_CLASSIC_FIREWALL",
	})
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if !strings.Contains(string(encoded), "networkFirewallPolicyEnforcementOrder") {
		t.Errorf("field missing from the response body: %s", encoded)
	}
}
