package persist

import (
	"os"
	"testing"
)

// State can include service account key material, so the file must not be
// readable by other users on the machine.
func TestStateFileIsOwnerOnly(t *testing.T) {
	withTempHome(t)

	if err := Save("perms", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(Path("perms"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("state file mode = %o, want 600", mode)
	}
}
