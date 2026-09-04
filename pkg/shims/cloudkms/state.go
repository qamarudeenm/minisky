package cloudkms

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. Losing a key ring is not just a missing resource: ciphertext
// written before the restart can never be decrypted again, because the AES key
// that produced it is gone. So the key material is persisted along with the
// metadata, into a file the helper creates owner-only.
//
// It is still kept out of API responses. Real KMS never returns key material,
// and `aesKey` stays unexported for exactly that reason — the shadow structs
// below exist to carry it to disk without putting it on the wire.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "cloudkms_state"

type persistedVersion struct {
	Version *CryptoKeyVersion `json:"version"`
	AesKey  []byte            `json:"aesKey,omitempty"`
}

type persistedKey struct {
	Key      *CryptoKey          `json:"key"`
	Versions []*persistedVersion `json:"versions,omitempty"`
}

type persistedRing struct {
	Ring *KeyRing                 `json:"ring"`
	Keys map[string]*persistedKey `json:"keys,omitempty"`
}

type persistedState struct {
	Store map[string]map[string]*persistedRing `json:"store"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: KMS] ignoring unreadable state: %v", err)
		return
	}

	rings := 0
	for location, storedRings := range stored.Store {
		ringsAt := make(map[string]*KeyRing, len(storedRings))
		for ringID, pr := range storedRings {
			ring := pr.Ring
			ring.keys = make(map[string]*CryptoKey, len(pr.Keys))
			for keyID, pk := range pr.Keys {
				key := pk.Key
				key.versions = make([]*CryptoKeyVersion, 0, len(pk.Versions))
				for _, pv := range pk.Versions {
					version := pv.Version
					version.aesKey = pv.AesKey
					key.versions = append(key.versions, version)
				}
				ring.keys[keyID] = key
			}
			ringsAt[ringID] = ring
			rings++
		}
		api.store[location] = ringsAt
	}
	if rings > 0 {
		log.Printf("[Shim: KMS] restored %d key ring(s)", rings)
	}
}

// persistIfMutated snapshots the state after a request that could have changed it.
func (api *API) persistIfMutated(r *http.Request) {
	if !persist.Mutating(r) {
		return
	}
	api.mu.RLock()
	defer api.mu.RUnlock()
	api.saveLocked()
}

// saveLocked writes the state out. Callers hold api.mu.
func (api *API) saveLocked() {
	state := persistedState{Store: make(map[string]map[string]*persistedRing, len(api.store))}
	for location, rings := range api.store {
		out := make(map[string]*persistedRing, len(rings))
		for ringID, ring := range rings {
			pr := &persistedRing{Ring: ring, Keys: make(map[string]*persistedKey, len(ring.keys))}
			for keyID, key := range ring.keys {
				pk := &persistedKey{Key: key}
				for _, version := range key.versions {
					pk.Versions = append(pk.Versions, &persistedVersion{Version: version, AesKey: version.aesKey})
				}
				pr.Keys[keyID] = pk
			}
			out[ringID] = pr
		}
		state.Store[location] = out
	}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: KMS] could not persist state: %v", err)
	}
}
