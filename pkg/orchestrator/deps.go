package orchestrator

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Dependency defines a system tool required by a MiniSky service.
type Dependency struct {
	ID          string
	Name        string
	Description string
	DownloadURL string
}

const (
	KindVersion = "v0.22.0"
	PackVersion = "v0.34.2"
)

// InstallDependency downloads and installs the matching binary for the system.
func (sm *ServiceManager) InstallDependency(id string) error {
	var dep Dependency
	var isArchive bool
	
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Map architectures
	if arch == "x86_64" { arch = "amd64" }
	if arch == "aarch64" { arch = "arm64" }

	switch id {
	case "kind":
		binaryName := "kind"
		if osName == "windows" {
			binaryName = "kind.exe"
		}
		dep = Dependency{
			ID:          "kind",
			Name:        binaryName,
			Description: "Kubernetes IN Docker - Local Kubernetes tool",
			DownloadURL: fmt.Sprintf("https://kind.sigs.k8s.io/dl/%s/kind-%s-%s", KindVersion, osName, arch),
		}
	case "pack":
		binaryName := "pack"
		ext := "tgz"
		isArchive = true
		if osName == "windows" {
			binaryName = "pack.exe"
			ext = "zip"
		} else if osName == "darwin" {
			ext = "tgz"
			osName = "macos" // pack uses 'macos' in URL
		}
		
		dep = Dependency{
			ID:          "pack",
			Name:        binaryName,
			Description: "Buildpacks CLI - Source-to-Image tool",
			DownloadURL: fmt.Sprintf("https://github.com/buildpacks/pack/releases/download/%s/pack-%s-%s.%s", PackVersion, PackVersion, osName, ext),
		}
	default:
		return fmt.Errorf("unsupported dependency: %s", id)
	}

	// 1. Create .minisky/bin if it doesn't exist
	binDir := filepath.Join(".minisky", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %v", err)
	}

	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("minisky-%s-dl", dep.ID))
	if isArchive {
		tempFile += "." + filepath.Ext(dep.DownloadURL)
	}
	
	log.Printf("[Deps] Downloading %s from %s...", dep.ID, dep.DownloadURL)

	// 2. Download
	resp, err := http.Get(dep.DownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// 3. Write to temp
	out, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return fmt.Errorf("failed to save download: %v", err)
	}

	// 4. Extract or Move
	targetPath := filepath.Join(binDir, dep.Name)
	if isArchive {
		log.Printf("[Deps] Extracting %s archive...", dep.ID)
		if strings.HasSuffix(tempFile, "tgz") || strings.HasSuffix(tempFile, "tar.gz") {
			// Use tar -xzf
			cmd := exec.Command("tar", "-xzf", tempFile, "-C", binDir, dep.Name)
			if err := cmd.Run(); err != nil {
				// Try without explicit filename in case of nesting
				exec.Command("tar", "-xzf", tempFile, "-C", binDir).Run()
			}
		} else {
			// Use unzip for windows
			exec.Command("unzip", "-o", tempFile, "-d", binDir).Run()
		}
	} else {
		// Just rename/move
		if err := os.Rename(tempFile, targetPath); err != nil {
			// Fallback: copy
			input, _ := os.ReadFile(tempFile)
			os.WriteFile(targetPath, input, 0755)
		}
	}
	
	// Ensure executable
	os.Chmod(targetPath, 0755)

	log.Printf("[Deps] ✅ Successfully installed %s to %s", dep.ID, targetPath)
	return nil
}

// GetLocalBinPath returns the absolute path to the local .minisky/bin folder.
func GetLocalBinPath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".minisky", "bin")
}

// GetKindBinaryName returns "kind" or "kind.exe" depending on the OS.
func GetKindBinaryName() string {
	if runtime.GOOS == "windows" {
		return "kind.exe"
	}
	return "kind"
}

// GetPackBinaryName returns "pack" or "pack.exe" depending on the OS.
func GetPackBinaryName() string {
	if runtime.GOOS == "windows" {
		return "pack.exe"
	}
	return "pack"
}
