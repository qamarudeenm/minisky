// Package persist stores small pieces of shim state under ~/.minisky so they
// survive a restart of the daemon.
//
// Without it the emulator forgets what it told a client a moment ago, and
// Terraform reads that as drift: a bucket whose location reverts to the
// emulator's default is destroyed and recreated on the next apply, taking every
// object in it, because location is a ForceNew attribute.
package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"minisky/pkg/config"
)

// Path returns the file a given state name is stored in.
func Path(name string) string {
	return filepath.Join(config.GetMiniskyDir(), name+".json")
}

// Load reads state into v. A missing file is not an error — it is what the
// first run looks like — and neither is an unreadable one: state is a cache of
// the emulator's own history, so a corrupt file should cost the history rather
// than prevent the daemon from starting.
func Load(name string, v any) error {
	data, err := os.ReadFile(Path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", Path(name), err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", Path(name), err)
	}
	return nil
}

// Save writes v as JSON, atomically.
//
// The write goes to a temporary file in the same directory and is renamed into
// place, so a daemon killed mid-write leaves the previous state intact rather
// than a truncated file that fails to parse on the next start.
func Save(name string, v any) error {
	path := Path(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "."+name+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", name, err)
	}
	tempName := temp.Name()

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(tempName)
		return fmt.Errorf("write %s: %w", tempName, err)
	}
	// Flush to disk before the rename, so a crash cannot leave the new name
	// pointing at an empty file.
	if err := temp.Sync(); err != nil {
		temp.Close()
		os.Remove(tempName)
		return fmt.Errorf("sync %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("close %s: %w", tempName, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
