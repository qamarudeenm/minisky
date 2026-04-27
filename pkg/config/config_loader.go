package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type ImageRegistry struct {
	Emulators map[string]EmulatorConfig `json:"emulators"`
	Compute   ComputeConfig             `json:"compute"`
	Sql       SqlConfig                 `json:"sql"`
	Serverless ServerlessConfig         `json:"serverless"`
	Dataproc   DataprocConfig           `json:"dataproc"`
	Memorystore MemorystoreConfig       `json:"memorystore"`
}

type EmulatorConfig struct {
	Name  string   `json:"name"`
	Image string   `json:"image"`
	Port  string   `json:"port"`
	Cmd   []string `json:"cmd"`
	Volume string   `json:"volume,omitempty"`
}

type ComputeConfig struct {
	OsImages     []OsImage `json:"os_images"`
	DefaultImage string    `json:"default_image"`
}

type OsImage struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Image string `json:"image"`
}

type SqlConfig struct {
	Postgres SqlEngineConfig `json:"postgres"`
	Mysql    SqlEngineConfig `json:"mysql"`
}

type SqlEngineConfig struct {
	Latest       string       `json:"latest"`
	Versions     []SqlVersion `json:"versions"`
	DefaultImage string       `json:"default_image"`
}

type SqlVersion struct {
	Version string `json:"version"`
	Label   string `json:"label"`
	Image   string `json:"image"`
}

type ServerlessConfig struct {
	Builder string `json:"builder"`
}

type DataprocConfig struct {
	Latest       string       `json:"latest"`
	Versions     []SqlVersion `json:"versions"` // Reusing SqlVersion as it has the same fields (version, label, image)
	DefaultImage string       `json:"default_image"`
	MasterPorts  []string     `json:"master_ports"`
}

type MemorystoreConfig struct {
	Redis     MemoryEngineConfig `json:"redis"`
	Memcached MemoryEngineConfig `json:"memcached"`
	Valkey    MemoryEngineConfig `json:"valkey"`
}

type MemoryEngineConfig struct {
	DefaultImage string          `json:"default_image"`
	Versions     []MemoryVersion `json:"versions"`
}

type MemoryVersion struct {
	Version string `json:"version"`
	Label   string `json:"label"`
	Image   string `json:"image"`
}

var (
	registry *ImageRegistry
	once     sync.Once
)

// GetImageRegistry returns the global image configuration.
// It lazily loads from configs/images.json on first call.
func GetImageRegistry() *ImageRegistry {
	once.Do(func() {
		registry = loadRegistry()
	})
	return registry
}

func loadRegistry() *ImageRegistry {
	// Try to find the config file relative to the project root
	paths := []string{
		"configs/images.json",
		"../configs/images.json",
		"../../configs/images.json",
	}

	var data []byte
	var err error
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		data, err = os.ReadFile(abs)
		if err == nil {
			log.Printf("[Config] Loaded image registry from %s", abs)
			break
		}
	}

	if err != nil {
		log.Printf("[Config] WARNING: Could not load images.json, using minimal defaults: %v", err)
		return fallbackRegistry()
	}

	var r ImageRegistry
	if err := json.Unmarshal(data, &r); err != nil {
		log.Printf("[Config] ERROR: Failed to parse images.json: %v", err)
		return fallbackRegistry()
	}

	return &r
}

func fallbackRegistry() *ImageRegistry {
	return &ImageRegistry{
		Emulators: make(map[string]EmulatorConfig),
		Compute: ComputeConfig{
			DefaultImage: "ubuntu:latest",
		},
		Sql: SqlConfig{
			Postgres: SqlEngineConfig{DefaultImage: "postgres:15"},
			Mysql:    SqlEngineConfig{DefaultImage: "mysql:8.0"},
		},
		Serverless: ServerlessConfig{
			Builder: "gcr.io/buildpacks/builder:google-24",
		},
		Dataproc: DataprocConfig{
			DefaultImage: "bitnami/spark:3.5",
			MasterPorts:  []string{"8080/tcp"},
		},
		Memorystore: MemorystoreConfig{
			Redis:     MemoryEngineConfig{DefaultImage: "redis:latest"},
			Memcached: MemoryEngineConfig{DefaultImage: "memcached:latest"},
			Valkey:    MemoryEngineConfig{DefaultImage: "valkey/valkey:latest"},
		},
	}
}
