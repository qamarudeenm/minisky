package orchestrator

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeTransport routes requests to a handler so tests can fake the Docker
// daemon's HTTP API without a real socket.
type fakeTransport struct {
	handler func(req *http.Request) (*http.Response, error)
	calls   []string // "METHOD path", in order, for assertions
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, req.Method+" "+req.URL.Path)
	return f.handler(req)
}

func jsonResp(status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestAttachedVPCNames(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		reqErr   bool
		expected []string
		wantErr  bool
	}{
		{
			name:     "default network maps to default vpc name",
			body:     `{"NetworkSettings":{"Networks":{"minisky-net":{}}}}`,
			expected: []string{"default"},
		},
		{
			name:     "vpc network maps back to its vpc name",
			body:     `{"NetworkSettings":{"Networks":{"minisky-vpc-shared":{}}}}`,
			expected: []string{"shared"},
		},
		{
			name:     "unrecognized networks are ignored",
			body:     `{"NetworkSettings":{"Networks":{"bridge":{}}}}`,
			expected: nil,
		},
		{
			name:     "no networks attached",
			body:     `{"NetworkSettings":{"Networks":{}}}`,
			expected: nil,
		},
		{
			name:    "request error is propagated, not swallowed as empty",
			reqErr:  true,
			wantErr: true,
		},
		{
			name:    "decode error is propagated, not swallowed as empty",
			body:    `not json`,
			wantErr: true,
		},
		{
			name:    "non-2xx status with a well-formed JSON error body is an error, not an empty result",
			body:    `{"message":"no such container"}`,
			status:  http.StatusNotFound,
			wantErr: true,
		},
		{
			name: "primary network (HostConfig.NetworkMode) is sorted to the front",
			body: `{"HostConfig":{"NetworkMode":"minisky-vpc-prod"},"NetworkSettings":{"Networks":{
				"minisky-net": {}, "minisky-vpc-prod": {}, "minisky-vpc-shared": {}
			}}}`,
			expected: []string{"prod", "default", "shared"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
				if tt.reqErr {
					return nil, fmt.Errorf("boom")
				}
				status := tt.status
				if status == 0 {
					status = http.StatusOK
				}
				return jsonResp(status, tt.body)
			}}
			sm := NewServiceManagerForTesting(ft)

			got, err := sm.attachedVPCNames("some-container")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil (got vpcNames %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("got %v, want %v", got, tt.expected)
			}
			if tt.name == "primary network (HostConfig.NetworkMode) is sorted to the front" {
				if len(got) == 0 || got[0] != "prod" {
					t.Errorf("got %v, want primary vpc %q first", got, "prod")
				}
				return
			}
			seen := map[string]bool{}
			for _, v := range got {
				seen[v] = true
			}
			for _, v := range tt.expected {
				if !seen[v] {
					t.Errorf("missing expected vpc name %q in %v", v, got)
				}
			}
		})
	}
}

func TestGetContainerIPForNetwork(t *testing.T) {
	body := `{"NetworkSettings":{"Networks":{
		"minisky-net": {"IPAddress": "172.18.0.2"},
		"minisky-vpc-shared": {"IPAddress": "172.19.0.5"}
	}}}`

	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, body)
	}}
	sm := NewServiceManagerForTesting(ft)

	ip, err := sm.GetContainerIPForNetwork("my-vm", "minisky-vpc-shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "172.19.0.5" {
		t.Errorf("got IP %q, want 172.19.0.5", ip)
	}

	// A network the container isn't attached to should come back empty, not error.
	ip, err = sm.GetContainerIPForNetwork("my-vm", "minisky-vpc-not-attached-yet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "" {
		t.Errorf("got IP %q for unattached network, want empty string", ip)
	}
}

func TestConnectContainerToNetwork(t *testing.T) {
	tests := []struct {
		name       string
		vpcName    string
		wantURLSeg string
	}{
		{name: "empty vpc uses shared network", vpcName: "", wantURLSeg: "/networks/minisky-net/connect"},
		{name: "default vpc uses shared network", vpcName: "default", wantURLSeg: "/networks/minisky-net/connect"},
		{name: "named vpc gets its own network", vpcName: "shared", wantURLSeg: "/networks/minisky-vpc-shared/connect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
				gotPath = req.URL.Path
				return jsonResp(http.StatusOK, "{}")
			}}
			sm := NewServiceManagerForTesting(ft)

			if err := sm.ConnectContainerToNetwork("my-vm", tt.vpcName); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPath != tt.wantURLSeg {
				t.Errorf("got path %q, want %q", gotPath, tt.wantURLSeg)
			}
		})
	}

	t.Run("docker error response is surfaced", func(t *testing.T) {
		ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(http.StatusInternalServerError, "network not found")
		}}
		sm := NewServiceManagerForTesting(ft)

		if err := sm.ConnectContainerToNetwork("my-vm", "shared"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("already-attached is treated as a no-op success, not an error", func(t *testing.T) {
		// A retried/duplicate ProvisionComputeVM call for a container that's already
		// on this network should not be treated as a failure that rolls back an
		// otherwise-healthy container.
		ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(http.StatusForbidden, `{"message":"endpoint with name my-vm already exists in network minisky-vpc-shared"}`)
		}}
		sm := NewServiceManagerForTesting(ft)

		if err := sm.ConnectContainerToNetwork("my-vm", "shared"); err != nil {
			t.Fatalf("expected already-attached to be treated as success, got error: %v", err)
		}
	})

	t.Run("transport error does not panic on nil response", func(t *testing.T) {
		ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		}}
		sm := NewServiceManagerForTesting(ft)

		if err := sm.ConnectContainerToNetwork("my-vm", "shared"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestAllowedPortsForVPC(t *testing.T) {
	sm := NewServiceManagerForTesting(&fakeTransport{})
	sm.RegisterFirewallRule("shared", FirewallEntry{Direction: "INGRESS", Action: "allow", Ports: []string{"80", "443"}})
	sm.RegisterFirewallRule("shared", FirewallEntry{Direction: "EGRESS", Action: "allow", Ports: []string{"9999"}})
	sm.RegisterFirewallRule("shared", FirewallEntry{Direction: "INGRESS", Action: "deny", Ports: []string{"22"}})

	got := sm.allowedPortsForVPC("shared")
	want := map[string]bool{"80": true, "443": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want ports %v", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected port %q in %v", p, got)
		}
	}
}

func TestApplyFirewallPortsToVPC_PrefersRegisteredVPCsOverDockerInspect(t *testing.T) {
	ft := dockerFakeForProvision(t, "")
	sm := NewServiceManagerForTesting(ft)

	// Provisioning records "my-vm" -> ["default", "shared"] in the in-memory
	// vpcRegistry.
	if err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", []string{"default", "shared"}, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sm.RegisterFirewallRule("shared", FirewallEntry{Direction: "INGRESS", Action: "allow", Ports: []string{"8080"}})

	ft.calls = nil // only count calls made by ApplyFirewallPortsToVPC itself
	if err := sm.ApplyFirewallPortsToVPC("shared", []string{"my-vm"}, []string{"ubuntu:latest"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var inspects int
	for _, c := range ft.calls {
		if strings.HasPrefix(c, "GET /containers/") && strings.HasSuffix(c, "/json") {
			inspects++
		}
	}
	// Exactly one container inspect: the post-recreate port-registry update. If the
	// registry weren't consulted, attachedVPCNames would add a second inspect
	// before delete.
	if inspects != 1 {
		t.Errorf("got %d container inspect calls, want 1 (registry should avoid the attachedVPCNames Docker round-trip): calls=%v", inspects, ft.calls)
	}
}

func TestApplyFirewallPortsToVPC_FallsBackToDockerInspectWhenNotRegistered(t *testing.T) {
	ft := dockerFakeForProvision(t, "")
	sm := NewServiceManagerForTesting(ft)
	// No ProvisionComputeVM call — vpcRegistry has no entry for "my-vm", simulating
	// a container that was provisioned before minisky's process last restarted.

	sm.RegisterFirewallRule("shared", FirewallEntry{Direction: "INGRESS", Action: "allow", Ports: []string{"8080"}})

	if err := sm.ApplyFirewallPortsToVPC("shared", []string{"my-vm"}, []string{"ubuntu:latest"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var inspects int
	for _, c := range ft.calls {
		if strings.HasPrefix(c, "GET /containers/") && strings.HasSuffix(c, "/json") {
			inspects++
		}
	}
	if inspects < 2 {
		t.Errorf("got %d container inspect calls, want at least 2 (attachedVPCNames fallback + post-recreate port registry): calls=%v", inspects, ft.calls)
	}
}

// dockerFakeForProvision fakes the sequence of Docker calls ProvisionComputeVM
// makes: create, start, connect (once per extra network), then an inspect for
// the port-registry update.
func dockerFakeForProvision(t *testing.T, failConnectForNetwork string) *fakeTransport {
	t.Helper()
	return &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == "POST" && strings.HasPrefix(req.URL.Path, "/containers/create"):
			return jsonResp(http.StatusCreated, `{"Id":"abc123"}`)
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/start"):
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/connect"):
			if failConnectForNetwork != "" && strings.Contains(req.URL.Path, failConnectForNetwork) {
				return jsonResp(http.StatusInternalServerError, "connect failed")
			}
			return jsonResp(http.StatusOK, "{}")
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/stop"):
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "DELETE":
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/json"):
			return jsonResp(http.StatusOK, `{"NetworkSettings":{"Ports":{}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}}
}

func TestProvisionComputeVM_EmptyVpcNamesDoesNotPanic(t *testing.T) {
	ft := dockerFakeForProvision(t, "")
	sm := NewServiceManagerForTesting(ft)

	if err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", nil, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range ft.calls {
		if strings.HasSuffix(c, "/connect") {
			t.Errorf("expected no /connect calls for an empty vpcNames, got call: %s", c)
		}
	}
}

func TestProvisionComputeVM_DedupsDuplicateVPCs(t *testing.T) {
	ft := dockerFakeForProvision(t, "")
	sm := NewServiceManagerForTesting(ft)

	// Two network interfaces both pointing at "default" used to try to
	// ConnectContainerToNetwork the primary network a second time, which a
	// real Docker daemon rejects as already-attached and rolls back an
	// otherwise-healthy container.
	err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", []string{"default", "default", "shared"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var connects int
	for _, c := range ft.calls {
		if strings.HasSuffix(c, "/connect") {
			connects++
		}
	}
	if connects != 1 {
		t.Errorf("got %d /connect calls, want 1 (duplicate 'default' entry should be deduped)", connects)
	}
	for _, c := range ft.calls {
		if strings.HasPrefix(c, "DELETE") {
			t.Errorf("did not expect a rollback DELETE call, calls were: %v", ft.calls)
		}
	}
}

func TestProvisionComputeVM_AttachesEveryNetwork(t *testing.T) {
	ft := dockerFakeForProvision(t, "")
	sm := NewServiceManagerForTesting(ft)

	err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", []string{"default", "shared", "prod"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var connects int
	for _, c := range ft.calls {
		if strings.HasSuffix(c, "/connect") {
			connects++
		}
	}
	if connects != 2 {
		t.Errorf("got %d /connect calls, want 2 (one per network past the primary)", connects)
	}
}

func TestProvisionComputeVM_RollsBackOnAttachFailure(t *testing.T) {
	ft := dockerFakeForProvision(t, "minisky-vpc-prod")
	sm := NewServiceManagerForTesting(ft)

	err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", []string{"default", "prod"}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error when a network attach fails, got nil")
	}

	var sawDelete bool
	for _, c := range ft.calls {
		if strings.HasPrefix(c, "DELETE") {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Errorf("expected a rollback DELETE call after attach failure, calls were: %v", ft.calls)
	}
}

func TestCreateAndDeleteVPCNetwork_UseSharedNamingScheme(t *testing.T) {
	var gotPaths []string
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		gotPaths = append(gotPaths, req.Method+" "+req.URL.Path)
		return jsonResp(http.StatusOK, "{}")
	}}
	sm := NewServiceManagerForTesting(ft)

	if err := sm.CreateVPCNetwork(context.Background(), "shared"); err != nil {
		t.Fatalf("CreateVPCNetwork: unexpected error: %v", err)
	}
	if err := sm.DeleteVPCNetwork(context.Background(), "shared"); err != nil {
		t.Fatalf("DeleteVPCNetwork: unexpected error: %v", err)
	}

	want := []string{"POST /networks/create", "DELETE /networks/" + DockerNetworkNameForVPC("shared")}
	if len(gotPaths) != len(want) || gotPaths[0] != want[0] || gotPaths[1] != want[1] {
		t.Errorf("got calls %v, want %v", gotPaths, want)
	}
}

func TestProvisionComputeVM_RollsBackOnStartFailure(t *testing.T) {
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/images/"):
			return jsonResp(http.StatusOK, "{}")
		case req.Method == "POST" && strings.HasPrefix(req.URL.Path, "/containers/create"):
			return jsonResp(http.StatusCreated, `{"Id":"abc123"}`)
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/start"):
			return jsonResp(http.StatusInternalServerError, "start failed")
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/stop"):
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "DELETE":
			return jsonResp(http.StatusNoContent, "")
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}}
	sm := NewServiceManagerForTesting(ft)

	err := sm.ProvisionComputeVM("my-vm", "ubuntu:latest", []string{"default"}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error when container start fails, got nil")
	}

	var sawDelete bool
	for _, c := range ft.calls {
		if strings.HasPrefix(c, "DELETE") {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Errorf("expected a rollback DELETE call after start failure, calls were: %v", ft.calls)
	}
}

func TestStopAndRemoveContainer(t *testing.T) {
	tests := []struct {
		name         string
		removeStatus int
		wantErr      bool
	}{
		{name: "successful removal", removeStatus: http.StatusNoContent, wantErr: false},
		{name: "container already gone (404) is not an error", removeStatus: http.StatusNotFound, wantErr: false},
		{name: "docker rejects removal, error is surfaced", removeStatus: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
				if req.Method == "DELETE" {
					return jsonResp(tt.removeStatus, "")
				}
				return jsonResp(http.StatusNoContent, "")
			}}
			sm := NewServiceManagerForTesting(ft)

			err := sm.StopAndRemoveContainer("my-vm")
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteComputeVM(t *testing.T) {
	tests := []struct {
		name         string
		removeStatus int
		wantErr      bool
	}{
		{name: "successful removal", removeStatus: http.StatusNoContent, wantErr: false},
		{name: "container already gone (404) is not an error", removeStatus: http.StatusNotFound, wantErr: false},
		{name: "docker rejects removal, error is surfaced", removeStatus: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
				if req.Method == "DELETE" {
					return jsonResp(tt.removeStatus, "")
				}
				return jsonResp(http.StatusNoContent, "")
			}}
			sm := NewServiceManagerForTesting(ft)

			err := sm.DeleteComputeVM("my-vm")
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Docker fixes a container's image and port bindings at create time, so adopting
// a container that already exists silently discards what the caller asked for —
// the VM comes back with no published ports while the API reports a fresh
// create. Provisioning must replace it instead.
func TestProvisionComputeVM_ReplacesExistingContainer(t *testing.T) {
	var created, removed int
	firstCreate := true

	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == "POST" && strings.HasPrefix(req.URL.Path, "/containers/create"):
			created++
			if firstCreate {
				firstCreate = false
				return jsonResp(http.StatusConflict, `{"message":"container name already in use"}`)
			}
			return jsonResp(http.StatusCreated, `{"Id":"abc123"}`)
		case req.Method == "DELETE":
			removed++
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/stop"):
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/start"):
			return jsonResp(http.StatusNoContent, "")
		case req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/json"):
			return jsonResp(http.StatusOK, `{"NetworkSettings":{"Ports":{}}}`)
		default:
			return jsonResp(http.StatusOK, "{}")
		}
	}}

	sm := NewServiceManagerForTesting(ft)
	if err := sm.ProvisionComputeVM("my-vm", "ubuntu:24.04", []string{"vpc"}, []string{"8080"}, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if removed == 0 {
		t.Error("the pre-existing container was adopted rather than removed")
	}
	if created != 2 {
		t.Errorf("got %d create calls, want 2 (the conflicting one, then the replacement)", created)
	}
}

// A VM with no route to the host cannot call the emulated GCP APIs at all,
// which is the entire point of running a workload inside one. Docker Desktop
// runs the daemon in its own VM, so the bridge gateway is not the host — the
// host-gateway mapping is what makes host.docker.internal resolve.
func TestProvisionComputeVM_GivesVMsARouteToTheHost(t *testing.T) {
	var createBody string
	ft := &fakeTransport{handler: func(req *http.Request) (*http.Response, error) {
		if req.Method == "POST" && strings.HasPrefix(req.URL.Path, "/containers/create") {
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				createBody = string(body)
			}
			return jsonResp(http.StatusCreated, `{"Id":"abc123"}`)
		}
		if req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/json") {
			return jsonResp(http.StatusOK, `{"NetworkSettings":{"Ports":{}}}`)
		}
		return jsonResp(http.StatusOK, "{}")
	}}

	sm := NewServiceManagerForTesting(ft)
	if err := sm.ProvisionComputeVM("my-vm", "ubuntu:24.04", []string{"vpc"}, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(createBody, "host.docker.internal:host-gateway") {
		t.Errorf("container create carries no host-gateway mapping: %s", createBody)
	}
}
