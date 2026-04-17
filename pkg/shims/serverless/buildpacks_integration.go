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
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// BuildpacksBackend manages local container builds via Google Cloud Buildpacks.
type BuildpacksBackend struct {
	enabled bool
	builder string // Buildpacks builder image to use
}

// DefaultBuilder is the Google-22 stack builder that mirrors GCP's build environment.
const DefaultBuilder = "gcr.io/buildpacks/builder:google-22"

// NewBuildpacksBackend returns a BuildpacksBackend. Only active when
// MINISKY_SERVERLESS_BACKEND=buildpacks is set.
func NewBuildpacksBackend() *BuildpacksBackend {
	enabled := strings.EqualFold(os.Getenv("MINISKY_SERVERLESS_BACKEND"), "buildpacks")
	builder := os.Getenv("MINISKY_BUILDPACKS_BUILDER")
	if builder == "" {
		builder = DefaultBuilder
	}

	b := &BuildpacksBackend{enabled: enabled, builder: builder}

	if enabled {
		if _, err := exec.LookPath("pack"); err != nil {
			log.Printf("[Buildpacks] WARNING: MINISKY_SERVERLESS_BACKEND=buildpacks but 'pack' CLI not found. Falling back to in-memory simulation.")
			b.enabled = false
		} else {
			log.Printf("[Buildpacks] ✅ Buildpacks integration ENABLED (builder: %s)", builder)
		}
	}
	return b
}

// Enabled reports whether Buildpacks backend is active.
func (b *BuildpacksBackend) Enabled() bool { return b.enabled }

// SetEnabled toggles the Buildpacks backend dynamically.
func (b *BuildpacksBackend) SetEnabled(enabled bool) error {
	b.enabled = enabled
	if enabled {
		if _, err := exec.LookPath("pack"); err != nil {
			b.enabled = false
			return fmt.Errorf("'pack' CLI not found, cannot enable")
		}
		log.Printf("[Buildpacks] dynamically ENABLED via UI")
	} else {
		log.Printf("[Buildpacks] dynamically DISABLED via UI")
	}
	return nil
}

// BuildFunction builds a Docker image for a Cloud Function source directory.
// Returns the image name and the local port where the container is started.
//
//   sourcePath — local directory containing the function source code.
//   functionName — used to name the Docker image (minisky-fn-<name>).
func (b *BuildpacksBackend) BuildFunction(functionName, sourcePath string) (imageRef string, err error) {
	if !b.enabled {
		return "", fmt.Errorf("buildpacks backend not enabled")
	}

	imageRef = fmt.Sprintf("minisky-fn-%s:local", sanitizeImageName(functionName))
	log.Printf("[Buildpacks] Building image %s from %s using builder %s", imageRef, sourcePath, b.builder)

	cmd := exec.Command("pack", "build", imageRef,
		"--path", sourcePath,
		"--builder", b.builder,
		"--trust-builder",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pack build failed for '%s': %w", functionName, err)
	}

	log.Printf("[Buildpacks] ✅ Image built: %s", imageRef)
	return imageRef, nil
}

// BuildService builds a Docker image for a Cloud Run service.
// Same mechanics as BuildFunction but uses different environment variable injection.
func (b *BuildpacksBackend) BuildService(serviceName, sourcePath string) (imageRef string, err error) {
	if !b.enabled {
		return "", fmt.Errorf("buildpacks backend not enabled")
	}

	imageRef = fmt.Sprintf("minisky-svc-%s:local", sanitizeImageName(serviceName))
	log.Printf("[Buildpacks] Building image %s from %s", imageRef, sourcePath)

	cmd := exec.Command("pack", "build", imageRef,
		"--path", sourcePath,
		"--builder", b.builder,
		"--trust-builder",
		// Inject the PORT env var as expected by Cloud Run containers
		"--env", "PORT=8080",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pack build failed for service '%s': %w", serviceName, err)
	}

	log.Printf("[Buildpacks] ✅ Service image built: %s", imageRef)
	return imageRef, nil
}

// StartContainer starts a Docker container from imageRef and returns its local URL.
// Used by the Serverless shim to provide a real invocation URL for the function/service.
func (b *BuildpacksBackend) StartContainer(name, imageRef string, envVars map[string]string) (localURL string, err error) {
	if !b.enabled {
		return "", fmt.Errorf("buildpacks backend not enabled")
	}

	containerName := "minisky-serverless-" + sanitizeImageName(name)
	args := []string{
		"run", "-d",
		"--rm",
		"--name", containerName,
		"-p", "0:8080", // allocate a random host port
		"--network", "minisky-net",
	}
	for k, v := range envVars {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, imageRef)

	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %w", err)
	}

	containerID := strings.TrimSpace(string(out))
	log.Printf("[Buildpacks] Started container %s (ID: %.12s)", containerName, containerID)

	// Discover the allocated host port
	portOut, err := exec.Command("docker", "port", containerID, "8080").Output()
	if err != nil {
		return "", fmt.Errorf("cannot discover container port: %w", err)
	}

	// portOut looks like "0.0.0.0:PORT\n"
	portStr := strings.TrimSpace(string(portOut))
	parts := strings.Split(portStr, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected docker port output: %s", portStr)
	}

	port := parts[len(parts)-1]
	localURL = "http://localhost:" + port
	log.Printf("[Buildpacks] ✅ Container listening at %s", localURL)
	return localURL, nil
}

// StopContainer stops a running minisky-serverless-* container.
func (b *BuildpacksBackend) StopContainer(name string) error {
	if !b.enabled {
		return nil
	}
	containerName := "minisky-serverless-" + sanitizeImageName(name)
	return exec.Command("docker", "stop", containerName).Run()
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
