package gke

// ─────────────────────────────────────────────────────────────────────────────
// Phase 6b — Kind Integration
//
// This file wires the GKE shim to a real local Kind (Kubernetes-in-Docker)
// cluster. When enabled via MINISKY_GKE_BACKEND=kind, cluster creation calls
// `kind create cluster` instead of only updating in-memory state.
//
// Prerequisites:
//   - kind CLI in PATH: https://kind.sigs.k8s.io/docs/user/quick-start/
//   - Docker daemon running
//
// Enable with: export MINISKY_GKE_BACKEND=kind
// ─────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// KindBackend drives real Kind cluster lifecycle.
type KindBackend struct {
	enabled bool
}

// NewKindBackend returns a KindBackend. It is only active when
// the MINISKY_GKE_BACKEND environment variable is set to "kind".
func NewKindBackend() *KindBackend {
	enabled := strings.EqualFold(os.Getenv("MINISKY_GKE_BACKEND"), "kind")
	if enabled {
		if _, err := exec.LookPath("kind"); err != nil {
			log.Printf("[KindBackend] WARNING: MINISKY_GKE_BACKEND=kind but 'kind' CLI not found in PATH. Falling back to in-memory simulation.")
			enabled = false
		} else {
			log.Printf("[KindBackend] ✅ Kind integration ENABLED — real local clusters will be provisioned.")
		}
	}
	return &KindBackend{enabled: enabled}
}

// Enabled reports whether Kind backend is active.
func (k *KindBackend) Enabled() bool { return k.enabled }

// SetEnabled toggles the Kind backend dynamically.
func (k *KindBackend) SetEnabled(enabled bool) error {
	k.enabled = enabled
	if enabled {
		if _, err := exec.LookPath("kind"); err != nil {
			k.enabled = false
			return fmt.Errorf("'kind' CLI not found, cannot enable")
		}
		log.Printf("[KindBackend] dynamically ENABLED via UI")
	} else {
		log.Printf("[KindBackend] dynamically DISABLED via UI")
	}
	return nil
}

// CreateCluster runs `kind create cluster --name <name>`.
// It blocks until the cluster is ready, then returns the kubeconfig path.
func (k *KindBackend) CreateCluster(clusterName string) (kubeconfigPath string, err error) {
	if !k.enabled {
		return "", fmt.Errorf("kind backend not enabled")
	}

	// Kind names must be lowercase alphanumeric+hyphens, max 63 chars.
	kindName := sanitizeKindName(clusterName)
	kubeconfigPath = fmt.Sprintf("/tmp/minisky-kubeconfig-%s.yaml", kindName)

	log.Printf("[KindBackend] Creating cluster: %s (kind name: %s)", clusterName, kindName)

	cmd := exec.Command("kind", "create", "cluster",
		"--name", kindName,
		"--kubeconfig", kubeconfigPath,
		"--wait", "120s", // wait up to 2 minutes for control plane
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kind create cluster failed: %w", err)
	}

	log.Printf("[KindBackend] ✅ Cluster '%s' ready. Kubeconfig: %s", kindName, kubeconfigPath)

	// Post-provisioning network hook
	// We dynamically attach the containers to minisky-net so they can communicate
	// with Compute VMs and BigQuery.
	log.Printf("[KindBackend] Linking cluster '%s' nodes to minisky-net...", kindName)
	go func() {
		out, err := exec.Command("docker", "ps", "--format", "{{.Names}}", "--filter", "name=^"+kindName+"-").Output()
		if err == nil {
			containers := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, cName := range containers {
				if cName == "" {
					continue
				}
				// Connect the node to minisky-net so it drops onto the same docker subnet as VMs
				exec.Command("docker", "network", "connect", "minisky-net", cName).Run()
				log.Printf("[KindBackend] Node '%s' connected to minisky-net", cName)
			}
		}
	}()

	return kubeconfigPath, nil
}

// DeleteCluster runs `kind delete cluster --name <name>`.
func (k *KindBackend) DeleteCluster(clusterName string) error {
	if !k.enabled {
		return fmt.Errorf("kind backend not enabled")
	}
	kindName := sanitizeKindName(clusterName)
	log.Printf("[KindBackend] Deleting cluster: %s", kindName)

	cmd := exec.Command("kind", "delete", "cluster", "--name", kindName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetEndpoint returns the Kind cluster's API server endpoint from the kubeconfig.
// In a real implementation this would parse the kubeconfig YAML.
// For now, Kind always exposes the API server on localhost:*
func (k *KindBackend) GetEndpoint(clusterName string) string {
	// Kind clusters are always reachable on 127.0.0.1 with a random high port.
	// The real endpoint can be read from the generated kubeconfig:
	//   kubectl --kubeconfig=<path> cluster-info
	// For now, return a predictable value the GKE shim will include in Cluster.Endpoint.
	return "127.0.0.1"
}

func sanitizeKindName(name string) string {
	result := strings.ToLower(name)
	result = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, result)
	if len(result) > 63 {
		result = result[:63]
	}
	return strings.Trim(result, "-")
}
