package cloudkms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/registry"
	"minisky/pkg/shims/logging"
)

func init() {
	registry.Register("cloudkms.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI()
	})
}

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

type CryptoKeyVersion struct {
	Name            string `json:"name"`
	State           string `json:"state"` // ENABLED | DISABLED | DESTROYED
	CreateTime      string `json:"createTime"`
	DestroyTime     string `json:"destroyTime,omitempty"`
	Algorithm       string `json:"algorithm"`
	aesKey          []byte // 32-byte AES-256 key, never serialised
}

type CryptoKey struct {
	Name            string             `json:"name"`
	Purpose         string             `json:"purpose"` // ENCRYPT_DECRYPT | ASYMMETRIC_SIGN | MAC
	CreateTime      string             `json:"createTime"`
	VersionTemplate map[string]any     `json:"versionTemplate,omitempty"`
	Labels          map[string]string  `json:"labels,omitempty"`
	mu              sync.Mutex
	versions        []*CryptoKeyVersion
}

type KeyRing struct {
	Name       string `json:"name"`
	CreateTime string `json:"createTime"`
	mu         sync.Mutex
	keys       map[string]*CryptoKey
}

type API struct {
	mu       sync.RWMutex
	// map[project/location] -> map[keyRingId] -> *KeyRing
	store    map[string]map[string]*KeyRing
	logAPI   *logging.API
}

func NewAPI() *API {
	return &API{store: make(map[string]map[string]*KeyRing)}
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	if logShim, ok := ctx.GetShim("logging.googleapis.com").(*logging.API); ok {
		api.logAPI = logShim
	}
}

func (api *API) pushLog(project, severity, resource, msg string) {
	if api.logAPI != nil {
		api.logAPI.PushLog(project, severity, "cloudkms_key", resource, msg)
	}
}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": msg},
	})
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

// Expected paths (after stripping /v1):
//   projects/{p}/locations/{loc}/keyRings
//   projects/{p}/locations/{loc}/keyRings/{kr}
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}:encrypt
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}:decrypt
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}/cryptoKeyVersions
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}/cryptoKeyVersions/{v}
//   projects/{p}/locations/{loc}/keyRings/{kr}/cryptoKeys/{ck}/cryptoKeyVersions/{v}:destroy

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Cloud KMS] %s %s", r.Method, r.URL.Path)
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	// Minimum: projects / {p} / locations / {loc} / keyRings
	if len(parts) < 5 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "keyRings" {
		jsonErr(w, http.StatusNotFound, "not found")
		return
	}

	project := parts[1]
	location := parts[3]
	locKey := project + "/" + location

	switch {
	// /keyRings
	case len(parts) == 5:
		switch r.Method {
		case http.MethodGet:
			api.listKeyRings(w, locKey)
		case http.MethodPost:
			krId := r.URL.Query().Get("keyRingId")
			api.createKeyRing(w, r, project, locKey, krId)
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// /keyRings/{kr}
	case len(parts) == 6:
		api.getKeyRing(w, locKey, parts[5])

	// /keyRings/{kr}/cryptoKeys
	case len(parts) == 7 && parts[6] == "cryptoKeys":
		switch r.Method {
		case http.MethodGet:
			api.listCryptoKeys(w, locKey, parts[5])
		case http.MethodPost:
			ckId := r.URL.Query().Get("cryptoKeyId")
			api.createCryptoKey(w, r, project, locKey, parts[5], ckId)
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// /keyRings/{kr}/cryptoKeys/{ck} or /cryptoKeys/{ck}:encrypt|decrypt
	case len(parts) == 8 && parts[6] == "cryptoKeys":
		raw := parts[7]
		if idx := strings.Index(raw, ":"); idx >= 0 {
			ckId, action := raw[:idx], raw[idx+1:]
			switch action {
			case "encrypt":
				api.encrypt(w, r, project, locKey, parts[5], ckId)
			case "decrypt":
				api.decrypt(w, r, project, locKey, parts[5], ckId)
			default:
				jsonErr(w, http.StatusNotFound, "unknown action: "+action)
			}
		} else {
			switch r.Method {
			case http.MethodGet:
				api.getCryptoKey(w, locKey, parts[5], raw)
			case http.MethodPatch:
				api.updateCryptoKey(w, r, locKey, parts[5], raw)
			default:
				jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}

	// /keyRings/{kr}/cryptoKeys/{ck}/cryptoKeyVersions
	case len(parts) == 9 && parts[6] == "cryptoKeys" && parts[8] == "cryptoKeyVersions":
		switch r.Method {
		case http.MethodGet:
			api.listCryptoKeyVersions(w, locKey, parts[5], parts[7])
		case http.MethodPost:
			api.createCryptoKeyVersion(w, project, locKey, parts[5], parts[7])
		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// /keyRings/{kr}/cryptoKeys/{ck}/cryptoKeyVersions/{v} or /{v}:destroy
	case len(parts) == 10 && parts[6] == "cryptoKeys" && parts[8] == "cryptoKeyVersions":
		raw := parts[9]
		if idx := strings.Index(raw, ":"); idx >= 0 {
			vId, action := raw[:idx], raw[idx+1:]
			if action == "destroy" {
				api.destroyCryptoKeyVersion(w, r, project, locKey, parts[5], parts[7], vId)
			} else {
				jsonErr(w, http.StatusNotFound, "unknown action: "+action)
			}
		} else {
			api.getCryptoKeyVersion(w, locKey, parts[5], parts[7], raw)
		}

	default:
		jsonErr(w, http.StatusNotFound, "route not found")
	}
}

// ---------------------------------------------------------------------------
// Key Ring CRUD
// ---------------------------------------------------------------------------

func (api *API) locationStore(locKey string) map[string]*KeyRing {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.store[locKey] == nil {
		api.store[locKey] = make(map[string]*KeyRing)
	}
	return api.store[locKey]
}

func (api *API) listKeyRings(w http.ResponseWriter, locKey string) {
	store := api.locationStore(locKey)
	api.mu.RLock()
	defer api.mu.RUnlock()
	list := make([]map[string]any, 0)
	for _, kr := range store {
		list = append(list, map[string]any{"name": kr.Name, "createTime": kr.CreateTime})
	}
	jsonOK(w, map[string]any{"keyRings": list, "totalSize": len(list)})
}

func (api *API) createKeyRing(w http.ResponseWriter, r *http.Request, project, locKey, krId string) {
	if krId == "" {
		jsonErr(w, http.StatusBadRequest, "keyRingId is required")
		return
	}
	store := api.locationStore(locKey)
	api.mu.Lock()
	if _, exists := store[krId]; exists {
		api.mu.Unlock()
		jsonErr(w, http.StatusConflict, "KeyRing already exists: "+krId)
		return
	}
	parts := strings.Split(locKey, "/")
	name := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s", parts[0], parts[1], krId)
	kr := &KeyRing{Name: name, CreateTime: nowStr(), keys: make(map[string]*CryptoKey)}
	store[krId] = kr
	api.mu.Unlock()
	api.pushLog(project, "INFO", name, "KeyRing created: "+name)
	jsonOK(w, map[string]any{"name": kr.Name, "createTime": kr.CreateTime})
}

func (api *API) getKeyRing(w http.ResponseWriter, locKey, krId string) {
	store := api.locationStore(locKey)
	api.mu.RLock()
	kr, ok := store[krId]
	api.mu.RUnlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "KeyRing not found: "+krId)
		return
	}
	jsonOK(w, map[string]any{"name": kr.Name, "createTime": kr.CreateTime})
}

// ---------------------------------------------------------------------------
// Crypto Key CRUD
// ---------------------------------------------------------------------------

func (api *API) getKeyRingOrErr(w http.ResponseWriter, locKey, krId string) *KeyRing {
	store := api.locationStore(locKey)
	api.mu.RLock()
	kr, ok := store[krId]
	api.mu.RUnlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "KeyRing not found: "+krId)
		return nil
	}
	return kr
}

func (api *API) listCryptoKeys(w http.ResponseWriter, locKey, krId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	defer kr.mu.Unlock()
	list := make([]map[string]any, 0)
	for _, ck := range kr.keys {
		list = append(list, cryptoKeyPublic(ck))
	}
	jsonOK(w, map[string]any{"cryptoKeys": list, "totalSize": len(list)})
}

func (api *API) createCryptoKey(w http.ResponseWriter, r *http.Request, project, locKey, krId, ckId string) {
	if ckId == "" {
		jsonErr(w, http.StatusBadRequest, "cryptoKeyId is required")
		return
	}
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}

	var body struct {
		Purpose         string            `json:"purpose"`
		VersionTemplate map[string]any    `json:"versionTemplate"`
		Labels          map[string]string `json:"labels"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Purpose == "" {
		body.Purpose = "ENCRYPT_DECRYPT"
	}

	kr.mu.Lock()
	if _, exists := kr.keys[ckId]; exists {
		kr.mu.Unlock()
		jsonErr(w, http.StatusConflict, "CryptoKey already exists: "+ckId)
		return
	}
	name := kr.Name + "/cryptoKeys/" + ckId
	ck := &CryptoKey{
		Name:            name,
		Purpose:         body.Purpose,
		CreateTime:      nowStr(),
		VersionTemplate: body.VersionTemplate,
		Labels:          body.Labels,
	}
	// Create primary version with local AES-256 key
	v := newCryptoKeyVersion(name, 1)
	ck.versions = append(ck.versions, v)
	kr.keys[ckId] = ck
	kr.mu.Unlock()

	api.pushLog(project, "INFO", name, fmt.Sprintf("CryptoKey created: %s (purpose: %s)", name, body.Purpose))
	jsonOK(w, cryptoKeyPublicWithPrimary(ck))
}

func (api *API) getCryptoKey(w http.ResponseWriter, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found: "+ckId)
		return
	}
	jsonOK(w, cryptoKeyPublicWithPrimary(ck))
}

func (api *API) updateCryptoKey(w http.ResponseWriter, r *http.Request, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found: "+ckId)
		return
	}
	var body struct {
		Labels          map[string]string `json:"labels"`
		VersionTemplate map[string]any    `json:"versionTemplate"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	ck.mu.Lock()
	if body.Labels != nil {
		ck.Labels = body.Labels
	}
	if body.VersionTemplate != nil {
		ck.VersionTemplate = body.VersionTemplate
	}
	ck.mu.Unlock()
	jsonOK(w, cryptoKeyPublicWithPrimary(ck))
}

// ---------------------------------------------------------------------------
// Crypto Key Versions
// ---------------------------------------------------------------------------

func newCryptoKeyVersion(ckName string, num int) *CryptoKeyVersion {
	aesKey := make([]byte, 32)
	rand.Read(aesKey)
	return &CryptoKeyVersion{
		Name:       fmt.Sprintf("%s/cryptoKeyVersions/%d", ckName, num),
		State:      "ENABLED",
		CreateTime: nowStr(),
		Algorithm:  "GOOGLE_SYMMETRIC_ENCRYPTION",
		aesKey:     aesKey,
	}
}

func (api *API) listCryptoKeyVersions(w http.ResponseWriter, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found")
		return
	}
	ck.mu.Lock()
	defer ck.mu.Unlock()
	list := make([]map[string]any, 0, len(ck.versions))
	for _, v := range ck.versions {
		list = append(list, cryptoKeyVersionPublic(v))
	}
	jsonOK(w, map[string]any{"cryptoKeyVersions": list, "totalSize": len(list)})
}

func (api *API) createCryptoKeyVersion(w http.ResponseWriter, project, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found")
		return
	}
	ck.mu.Lock()
	num := len(ck.versions) + 1
	v := newCryptoKeyVersion(ck.Name, num)
	ck.versions = append(ck.versions, v)
	ck.mu.Unlock()
	api.pushLog(project, "INFO", ck.Name, fmt.Sprintf("Version %d created", num))
	jsonOK(w, cryptoKeyVersionPublic(v))
}

func (api *API) getCryptoKeyVersion(w http.ResponseWriter, locKey, krId, ckId, vId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found")
		return
	}
	v := resolveVersion(ck, vId)
	if v == nil {
		jsonErr(w, http.StatusNotFound, "CryptoKeyVersion not found: "+vId)
		return
	}
	jsonOK(w, cryptoKeyVersionPublic(v))
}

func (api *API) destroyCryptoKeyVersion(w http.ResponseWriter, r *http.Request, project, locKey, krId, ckId, vId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found")
		return
	}
	v := resolveVersion(ck, vId)
	if v == nil {
		jsonErr(w, http.StatusNotFound, "CryptoKeyVersion not found: "+vId)
		return
	}
	ck.mu.Lock()
	v.State = "DESTROYED"
	v.DestroyTime = nowStr()
	v.aesKey = nil // Wipe key material
	ck.mu.Unlock()
	api.pushLog(project, "WARNING", ck.Name, "CryptoKeyVersion destroyed: "+v.Name)
	jsonOK(w, cryptoKeyVersionPublic(v))
}

// ---------------------------------------------------------------------------
// Encrypt / Decrypt (AES-256-GCM)
// ---------------------------------------------------------------------------

func (api *API) encrypt(w http.ResponseWriter, r *http.Request, project, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found: "+ckId)
		return
	}

	var body struct {
		Plaintext string `json:"plaintext"` // base64-encoded
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Plaintext == "" {
		jsonErr(w, http.StatusBadRequest, "plaintext (base64) is required")
		return
	}

	plaintext, err := base64.StdEncoding.DecodeString(body.Plaintext)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid base64 plaintext")
		return
	}

	v := primaryVersion(ck)
	if v == nil || v.State != "ENABLED" || v.aesKey == nil {
		jsonErr(w, http.StatusFailedDependency, "no enabled primary key version")
		return
	}

	ciphertext, err := aesGCMEncrypt(v.aesKey, plaintext)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "encryption failed: "+err.Error())
		return
	}

	api.pushLog(project, "INFO", ck.Name, "Encrypt operation performed")
	jsonOK(w, map[string]any{
		"name":           v.Name,
		"ciphertext":     base64.StdEncoding.EncodeToString(ciphertext),
		"ciphertextCrc32c": 0, // Not implemented; CRC32c verification is optional
	})
}

func (api *API) decrypt(w http.ResponseWriter, r *http.Request, project, locKey, krId, ckId string) {
	kr := api.getKeyRingOrErr(w, locKey, krId)
	if kr == nil {
		return
	}
	kr.mu.Lock()
	ck, ok := kr.keys[ckId]
	kr.mu.Unlock()
	if !ok {
		jsonErr(w, http.StatusNotFound, "CryptoKey not found: "+ckId)
		return
	}

	var body struct {
		Ciphertext string `json:"ciphertext"` // base64-encoded
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Ciphertext == "" {
		jsonErr(w, http.StatusBadRequest, "ciphertext (base64) is required")
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(body.Ciphertext)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid base64 ciphertext")
		return
	}

	// Try all ENABLED versions (most recent first) for decryption
	ck.mu.Lock()
	versions := make([]*CryptoKeyVersion, len(ck.versions))
	copy(versions, ck.versions)
	ck.mu.Unlock()

	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if v.State != "ENABLED" || v.aesKey == nil {
			continue
		}
		plaintext, err := aesGCMDecrypt(v.aesKey, ciphertext)
		if err == nil {
			api.pushLog(project, "INFO", ck.Name, "Decrypt operation performed")
			jsonOK(w, map[string]any{
				"plaintext":          base64.StdEncoding.EncodeToString(plaintext),
				"usedPrimary":        i == len(versions)-1,
				"protectionLevel":    "SOFTWARE",
			})
			return
		}
	}

	jsonErr(w, http.StatusBadRequest, "decryption failed: ciphertext is invalid or key has been rotated/destroyed")
}

// ---------------------------------------------------------------------------
// AES-256-GCM helpers
// ---------------------------------------------------------------------------

func aesGCMEncrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Prepend nonce to ciphertext so we can extract it on decrypt
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func aesGCMDecrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func resolveVersion(ck *CryptoKey, ref string) *CryptoKeyVersion {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	if ref == "1" || ref == "" {
		if len(ck.versions) == 0 {
			return nil
		}
	}
	for _, v := range ck.versions {
		if strings.HasSuffix(v.Name, "/"+ref) {
			return v
		}
	}
	return nil
}

func primaryVersion(ck *CryptoKey) *CryptoKeyVersion {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	for i := len(ck.versions) - 1; i >= 0; i-- {
		if ck.versions[i].State == "ENABLED" {
			return ck.versions[i]
		}
	}
	return nil
}

func cryptoKeyVersionPublic(v *CryptoKeyVersion) map[string]any {
	m := map[string]any{
		"name":       v.Name,
		"state":      v.State,
		"createTime": v.CreateTime,
		"algorithm":  v.Algorithm,
	}
	if v.DestroyTime != "" {
		m["destroyTime"] = v.DestroyTime
	}
	return m
}

func cryptoKeyPublic(ck *CryptoKey) map[string]any {
	return map[string]any{
		"name":            ck.Name,
		"purpose":         ck.Purpose,
		"createTime":      ck.CreateTime,
		"versionTemplate": ck.VersionTemplate,
		"labels":          ck.Labels,
	}
}

func cryptoKeyPublicWithPrimary(ck *CryptoKey) map[string]any {
	m := cryptoKeyPublic(ck)
	v := primaryVersion(ck)
	if v != nil {
		m["primary"] = cryptoKeyVersionPublic(v)
	}
	return m
}
