//go:build !windows

package orchestrator

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func resolveDockerSocket() string {
	// 1. Explicit DOCKER_HOST env var
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return strings.TrimPrefix(host, "unix://")
	}
	// 2. Probe known locations in priority order
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".docker", "run", "docker.sock"),     // Docker Desktop (current macOS)
		filepath.Join(os.Getenv("HOME"), ".docker", "desktop", "docker.sock"), // Docker Desktop (Linux/Mac)
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock"),            // Rootless Docker
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "podman", "podman.sock"),  // Podman
		"/var/run/docker.sock", // Classic system Docker
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/var/run/docker.sock"
}

func (sm *ServiceManager) dialDocker(ctx context.Context, _, _ string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", sm.sockPath)
}
