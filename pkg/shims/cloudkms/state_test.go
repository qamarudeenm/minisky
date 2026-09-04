package cloudkms

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func call(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	var decoded map[string]interface{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec.Code, decoded
}

// Losing a key ring is not a missing resource, it is unreadable data: every
// ciphertext written before the restart becomes undecryptable, because the AES
// key that produced it is gone. Persisting the metadata alone would leave the
// key present and the ciphertext still lost, which is worse — the failure moves
// from "key not found" to a decrypt that cannot succeed.
func TestCiphertextStaysDecryptableAcrossARestart(t *testing.T) {
	isolate(t)

	const ring = "/v1/projects/probe/locations/global/keyRings"
	api := NewAPI()
	if code, _ := call(t, api, http.MethodPost, ring+"?keyRingId=app", `{}`); code != http.StatusOK {
		t.Fatalf("create key ring = %d, want 200", code)
	}
	if code, _ := call(t, api, http.MethodPost, ring+"/app/cryptoKeys?cryptoKeyId=data",
		`{"purpose":"ENCRYPT_DECRYPT"}`); code != http.StatusOK {
		t.Fatalf("create crypto key = %d, want 200", code)
	}

	// "aGVsbG8=" is "hello".
	code, enc := call(t, api, http.MethodPost, ring+"/app/cryptoKeys/data:encrypt",
		`{"plaintext":"aGVsbG8="}`)
	if code != http.StatusOK {
		t.Fatalf("encrypt = %d, want 200", code)
	}
	ciphertext, _ := enc["ciphertext"].(string)
	if ciphertext == "" {
		t.Fatalf("no ciphertext returned: %v", enc)
	}

	restarted := NewAPI()
	code, dec := call(t, restarted, http.MethodPost, ring+"/app/cryptoKeys/data:decrypt",
		`{"ciphertext":"`+ciphertext+`"}`)
	if code != http.StatusOK {
		t.Fatalf("decrypt after restart = %d, want 200 — the key material was lost", code)
	}
	if dec["plaintext"] != "aGVsbG8=" {
		t.Errorf("plaintext = %v after restart, want the original", dec["plaintext"])
	}
}

// Key material must never reach a client, which is what makes it safe to hold
// unexported — persisting it must not change that.
func TestKeyMaterialIsNotReturnedToCallers(t *testing.T) {
	isolate(t)

	const ring = "/v1/projects/probe/locations/global/keyRings"
	api := NewAPI()
	call(t, api, http.MethodPost, ring+"?keyRingId=app", `{}`)
	call(t, api, http.MethodPost, ring+"/app/cryptoKeys?cryptoKeyId=data", `{"purpose":"ENCRYPT_DECRYPT"}`)

	req := httptest.NewRequest(http.MethodGet, ring+"/app/cryptoKeys/data", nil)
	rec := httptest.NewRecorder()
	NewAPI().ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "aesKey") {
		t.Errorf("key material appeared in an API response: %s", rec.Body.String())
	}
}
