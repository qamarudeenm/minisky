package persist

import (
	"os"
	"path/filepath"
	"testing"
)

type state struct {
	Buckets map[string]string `json:"buckets"`
}

func withTempHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestRoundTrip(t *testing.T) {
	withTempHome(t)

	if err := Save("probe", state{Buckets: map[string]string{"raw": "US"}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	var loaded state
	if err := Load("probe", &loaded); err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Buckets["raw"] != "US" {
		t.Errorf("loaded = %v, want the location that was saved", loaded.Buckets)
	}
}

// A first run has no file, which is not a failure.
func TestLoadMissingFileIsNotAnError(t *testing.T) {
	withTempHome(t)

	var loaded state
	if err := Load("never-written", &loaded); err != nil {
		t.Errorf("a missing file should read as empty state, got %v", err)
	}
	if loaded.Buckets != nil {
		t.Errorf("expected nothing to be loaded, got %v", loaded.Buckets)
	}
}

// A truncated or hand-edited file costs the history, not the daemon.
func TestLoadReportsCorruptionWithoutPanicking(t *testing.T) {
	withTempHome(t)

	if err := os.MkdirAll(filepath.Dir(Path("broken")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("broken"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var loaded state
	if err := Load("broken", &loaded); err == nil {
		t.Error("expected a parse error the caller can log")
	}
}

// The write must be atomic: a reader never sees a half-written file, and a
// failed write leaves the previous state readable.
func TestSaveIsAtomic(t *testing.T) {
	withTempHome(t)

	if err := Save("atomic", state{Buckets: map[string]string{"first": "US"}}); err != nil {
		t.Fatal(err)
	}
	if err := Save("atomic", state{Buckets: map[string]string{"second": "EU"}}); err != nil {
		t.Fatal(err)
	}

	var loaded state
	if err := Load("atomic", &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Buckets["second"] != "EU" || loaded.Buckets["first"] != "" {
		t.Errorf("the second write should have replaced the first, got %v", loaded.Buckets)
	}

	// No temporary files are left behind for the next start to trip over.
	entries, err := os.ReadDir(filepath.Dir(Path("atomic")))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("left a temporary file behind: %s", e.Name())
		}
	}
}
