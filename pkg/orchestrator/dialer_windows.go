//go:build windows

package orchestrator

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

func resolveDockerSocket() string {
	// 1. Explicit DOCKER_HOST env var
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		if strings.HasPrefix(host, "npipe://") {
			return host
		}
		return strings.TrimPrefix(host, "unix://")
	}

	// 2. Default Windows named pipe
	defaultPipe := `//./pipe/docker_engine`
	return defaultPipe
}

func (sm *ServiceManager) dialDocker(ctx context.Context, _, _ string) (net.Conn, error) {
	if strings.HasPrefix(sm.sockPath, "//./pipe/") {
		// Windows Named Pipe
		return winio.DialPipeContext(ctx, sm.sockPath)
	}
	// Fallback to TCP if it looks like an IP (though usually handled by default transport)
	var d net.Dialer
	return d.DialContext(ctx, "tcp", sm.sockPath)
}
