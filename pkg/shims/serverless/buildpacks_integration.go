package serverless

// ─────────────────────────────────────────────────────────────────────────────
// Phase 9b — Google Cloud Buildpacks Integration (Cloud Functions & Cloud Run)
//
// This file wires the Serverless shim to real local container builds using
// Google Cloud Buildpacks (https://buildpacks.io / cloud.google.com/buildpacks).
//
// When enabled via MINISKY_SERVERLESS_BACKEND=buildpacks, function/service
// creation will:
//   1. Download source code (from GCS stub or local path).
//   2. Run `pack build` to create a Docker image using Google Buildpacks.
//   3. Start the resulting container and wire its URL into the function/service.
//
// Prerequisites:
//   - pack CLI: https://buildpacks.io/docs/tools/pack/
//   - Docker daemon running
//   - Google Buildpacks builder: gcr.io/buildpacks/builder:google-22
//
// Enable with: export MINISKY_SERVERLESS_BACKEND=buildpacks
// ─────────────────────────────────────────────────────────────────────────────

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"minisky/pkg/orchestrator"
)

// BuildpacksBackend manages local container builds via Google Cloud Buildpacks.
type BuildpacksBackend struct {
	enabled    bool
	builder    string // Buildpacks builder image to use
	apiVersion string // Docker Engine API version negotiated with the daemon
	logStore   map[string]*bytes.Buffer
	logMu      sync.RWMutex
}

// DefaultBuilder is the Google-24 stack builder that mirrors GCP's latest build environment.
const DefaultBuilder = "gcr.io/buildpacks/builder:google-24"

// NewBuildpacksBackend returns a BuildpacksBackend. Only active when
// MINISKY_SERVERLESS_BACKEND=buildpacks is set. apiVersion is the Docker Engine
// API version negotiated with the running daemon; it is passed to the `pack`
// subprocess so its embedded Docker client speaks a version the daemon accepts.
func NewBuildpacksBackend(apiVersion string) *BuildpacksBackend {
	enabled := strings.EqualFold(os.Getenv("MINISKY_SERVERLESS_BACKEND"), "buildpacks")
	builder := os.Getenv("MINISKY_BUILDPACKS_BUILDER")
	if builder == "" {
		builder = DefaultBuilder
	}

	b := &BuildpacksBackend{
		enabled:    enabled,
		builder:    builder,
		apiVersion: apiVersion,
		logStore:   make(map[string]*bytes.Buffer),
	}

	if enabled {
		binPath := orchestrator.GetPackBinaryName()
		if _, err := exec.LookPath(binPath); err != nil {
			// Fallback: check local bin
			localPack := filepath.Join(orchestrator.GetLocalBinPath(), binPath)
			if _, err := os.Stat(localPack); err == nil {
				binPath = localPack
			} else {
				log.Printf("[Buildpacks] WARNING: MINISKY_SERVERLESS_BACKEND=buildpacks but 'pack' CLI not found. Falling back to in-memory simulation.")
				b.enabled = false
			}
		}

		if b.enabled {
			ver := detectPackVersion(binPath)
			if ver == "" || strings.HasPrefix(ver, "0.0.0") {
				log.Printf("[Buildpacks] ⚠️  'pack' at %s reports version %q, not the pinned %s release. It may be a locally built or wrong-arch binary; reinstall via MiniSky to get the official CLI.", binPath, ver, orchestrator.PackVersion)
			}
			log.Printf("[Buildpacks] ✅ Buildpacks integration ENABLED (pack %s, builder: %s)", ver, builder)
		}
	}
	return b
}

// detectPackVersion returns the first line of `pack version`, or "" on failure.
func detectPackVersion(binPath string) string {
	out, err := exec.Command(binPath, "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// Enabled reports whether Buildpacks backend is active.
func (b *BuildpacksBackend) Enabled() bool { return b.enabled }

// SetEnabled toggles the Buildpacks backend dynamically.
func (b *BuildpacksBackend) SetEnabled(enabled bool) error {
	b.enabled = enabled
	if enabled {
		binPath := orchestrator.GetPackBinaryName()
		if _, err := exec.LookPath(binPath); err != nil {
			localPack := filepath.Join(orchestrator.GetLocalBinPath(), binPath)
			if _, err := os.Stat(localPack); err != nil {
				b.enabled = false
				return fmt.Errorf("'pack' CLI not found, cannot enable")
			}
		}
		log.Printf("[Buildpacks] dynamically ENABLED via UI")
	} else {
		log.Printf("[Buildpacks] dynamically DISABLED via UI")
	}
	return nil
}

func (b *BuildpacksBackend) GetLogs(name string) string {
	b.logMu.RLock()
	defer b.logMu.RUnlock()
	if buf, ok := b.logStore[name]; ok {
		return buf.String()
	}
	return ""
}

// buildEnv returns the environment for the `pack` subprocess, ensuring
// DOCKER_API_VERSION carries the daemon-negotiated version so pack's embedded
// Docker client speaks a version the engine accepts. An explicit value already
// present in the environment (user override or daemon-set) is preserved.
func (b *BuildpacksBackend) buildEnv() []string {
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, "DOCKER_API_VERSION=") {
			return env
		}
	}
	version := b.apiVersion
	if version == "" {
		version = orchestrator.PreferredDockerAPIVersion
	}
	return append(env, "DOCKER_API_VERSION="+version)
}

// BuildFunction builds a Docker image for a Cloud Function source directory.
func (b *BuildpacksBackend) BuildFunction(functionName, sourcePath, entryPoint string) (imageRef string, err error) {
	if !b.enabled {
		return "", fmt.Errorf("buildpacks backend not enabled")
	}

	imageRef = fmt.Sprintf("minisky-fn-%s:local", sanitizeImageName(functionName))
	log.Printf("[Buildpacks] Building image %s from %s using builder %s", imageRef, sourcePath, b.builder)

	binPath := orchestrator.GetPackBinaryName()
	localPack := filepath.Join(orchestrator.GetLocalBinPath(), binPath)
	if _, err := os.Stat(localPack); err == nil {
		binPath = localPack
	}

	// Force shell to see the version and allow internet access for dependencies
	// Crucially, we pass GOOGLE_FUNCTION_TARGET at build time so the buildpacks 
	// can generate the correct entrypoint metadata.
	cmdArgs := []string{"-c", fmt.Sprintf("%s build %s --path %s --builder %s --trust-builder --network host --env GOOGLE_FUNCTION_TARGET=%s --env GOOGLE_FUNCTION_SIGNATURE_TYPE=http", binPath, imageRef, sourcePath, b.builder, entryPoint)}
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Env = b.buildEnv()

	buf := new(bytes.Buffer)
	b.logMu.Lock()
	b.logStore[functionName] = buf
	b.logMu.Unlock()
	cmd.Stdout = io.MultiWriter(os.Stdout, buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, buf)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pack build failed for '%s': %w", functionName, err)
	}

	log.Printf("[Buildpacks] ✅ Image built: %s", imageRef)
	return imageRef, nil
}

// BuildService builds a Docker image for a Cloud Run service.
func (b *BuildpacksBackend) BuildService(serviceName, sourcePath string) (imageRef string, err error) {
	if !b.enabled {
		return "", fmt.Errorf("buildpacks backend not enabled")
	}

	imageRef = fmt.Sprintf("minisky-svc-%s:local", sanitizeImageName(serviceName))
	log.Printf("[Buildpacks] Building image %s from %s", imageRef, sourcePath)

	binPath := orchestrator.GetPackBinaryName()
	localPack := filepath.Join(orchestrator.GetLocalBinPath(), binPath)
	if _, err := os.Stat(localPack); err == nil {
		binPath = localPack
	}

	cmdArgs := []string{"-c", fmt.Sprintf("%s build %s --path %s --builder %s --trust-builder --network host --env PORT=8080", binPath, imageRef, sourcePath, b.builder)}
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Env = b.buildEnv()

	buf := new(bytes.Buffer)
	b.logMu.Lock()
	b.logStore[serviceName] = buf
	b.logMu.Unlock()
	cmd.Stdout = io.MultiWriter(os.Stdout, buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, buf)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pack build failed for service '%s': %w", serviceName, err)
	}

	log.Printf("[Buildpacks] ✅ Service image built: %s", imageRef)
	return imageRef, nil
}

// DownloadSourceFromGCS retrieves a zip archive from the storage emulator and extracts it.
func (b *BuildpacksBackend) DownloadSourceFromGCS(bucket, object string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "minisky-source-")
	if err != nil {
		return "", err
	}

	// Hit the GCS emulator (linked to host port 4443)
	url := fmt.Sprintf("http://localhost:4443/storage/v1/b/%s/o/%s?alt=media", bucket, object)
	log.Printf("[Buildpacks] Downloading source from GCS: %s", url)

	cmd := exec.Command("curl", "-L", "-o", tmpDir+"/source.zip", url)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to download source from GCS: %w", err)
	}

	// Extract
	unzipCmd := exec.Command("unzip", "-q", "-d", tmpDir, tmpDir+"/source.zip")
	unzipCmd.Run() // ignore errors if it wasn't a zip (might be a raw dir if we were fancy)

	return tmpDir, nil
}

func sanitizeImageName(name string) string {
	result := strings.ToLower(name)
	result = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, result)
	return strings.Trim(result, "-")
}
