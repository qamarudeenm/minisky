package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Validator — the main contract enforcement engine
// ─────────────────────────────────────────────────────────────────────────────

// Validator enforces GCP Discovery Document contracts on every incoming request.
// It is injected into the ProxyRouter and called before any shim dispatch.
type Validator struct {
	// rulesIndex maps service domain → its compiled MethodSchema list.
	rulesIndex map[string]*ServiceSchema
}

// NewValidator loads the embedded Discovery rules and returns a ready Validator.
func NewValidator() *Validator {
	idx := make(map[string]*ServiceSchema, len(embeddedRules))
	for i := range embeddedRules {
		s := &embeddedRules[i]
		idx[s.Domain] = s
		log.Printf("[Validator] Loaded schema for domain: %s (%d methods)",
			s.Domain, len(s.Methods))
	}
	return &Validator{rulesIndex: idx}
}

// ValidateRequest checks the request against all applicable Discovery rules.
// Returns false if the request is invalid and writes a GCP-shaped error response.
// Returns true if the request passes all checks.
func (v *Validator) ValidateRequest(w http.ResponseWriter, r *http.Request) bool {
	domain := r.Host

	// ── 1. Skip validation for domains with no embedded rules ────────────────
	svc, ok := v.rulesIndex[domain]
	if !ok {
		// No schema for this domain — pass through (lazy Docker backends, etc.)
		return true
	}

	// ── 2. Find the matching MethodSchema ────────────────────────────────────
	rule := v.matchRule(svc, r.Method, r.URL.Path)
	if rule == nil {
		// No specific rule for this (method, path) pair — allow it.
		return true
	}

	// ── 3. Content-Type check ─────────────────────────────────────────────────
	if rule.ContentType != "" && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		ct := r.Header.Get("Content-Type")
		// Allow "application/json; charset=utf-8" etc.
		if !strings.HasPrefix(ct, rule.ContentType) {
			log.Printf("[Validator] 415 for %s %s — got Content-Type: %q", r.Method, r.URL.Path, ct)
			return v.emitError(w, 415, "INVALID_ARGUMENT",
				fmt.Sprintf("Unsupported Content-Type '%s'. Expected '%s'.", ct, rule.ContentType))
		}
	}

	// ── 4. Required query parameters ──────────────────────────────────────────
	for _, qp := range rule.RequiredQuery {
		if r.URL.Query().Get(qp) == "" {
			log.Printf("[Validator] 400 for %s %s — missing query param: %s", r.Method, r.URL.Path, qp)
			return v.emitError(w, 400, "INVALID_ARGUMENT",
				fmt.Sprintf("Query parameter '%s' is required.", qp))
		}
	}

	// ── 5. Required body fields ───────────────────────────────────────────────
	if len(rule.RequiredBody) > 0 {
		// Read body once, then restore it so the downstream shim can read it too.
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return v.emitError(w, 400, "INVALID_ARGUMENT", "Cannot read request body: "+err.Error())
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))

		// Empty body when we expect one
		if len(raw) == 0 {
			first := rule.RequiredBody[0]
			msg := first.Message
			if msg == "" {
				msg = fmt.Sprintf("Field '%s' is required but request body is empty.", first.Path)
			}
			log.Printf("[Validator] 400 for %s %s — empty body", r.Method, r.URL.Path)
			return v.emitError(w, 400, "INVALID_ARGUMENT", msg)
		}

		// Parse JSON body
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			log.Printf("[Validator] 400 for %s %s — body not valid JSON: %v", r.Method, r.URL.Path, err)
			return v.emitError(w, 400, "INVALID_ARGUMENT",
				"Request body is not valid JSON: "+err.Error())
		}

		// Check each required field
		for _, req := range rule.RequiredBody {
			val := getNestedField(body, req.Path)
			if val == nil {
				msg := req.Message
				if msg == "" {
					msg = fmt.Sprintf("Field '%s' is required.", req.Path)
				}
				log.Printf("[Validator] 400 for %s %s — missing field: %s", r.Method, r.URL.Path, req.Path)
				return v.emitError(w, 400, "INVALID_ARGUMENT", msg)
			}

			// Type check (only when a type is specified)
			if req.Type != "" {
				if typeErr := checkFieldType(req.Path, val, req.Type); typeErr != "" {
					log.Printf("[Validator] 400 for %s %s — type error: %s", r.Method, r.URL.Path, typeErr)
					return v.emitError(w, 400, "INVALID_ARGUMENT", typeErr)
				}
			}
		}
	}

	log.Printf("[Validator] ✅ Contract validated: %s %s@%s", r.Method, r.URL.Path, domain)
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// matchRule — glob-based method dispatcher
// ─────────────────────────────────────────────────────────────────────────────

// matchRule finds the first MethodSchema whose HTTPMethod and PathGlob match.
// The glob uses "*" to mean "any single non-empty path segment".
func (v *Validator) matchRule(svc *ServiceSchema, httpMethod, urlPath string) *MethodSchema {
	for i := range svc.Methods {
		m := &svc.Methods[i]
		if !strings.EqualFold(m.HTTPMethod, httpMethod) {
			continue
		}
		if globMatch(m.PathGlob, urlPath) {
			return m
		}
	}
	return nil
}

// globMatch returns true if path matches glob where each "*" matches any single
// URL path segment (non-empty, no slash).
func globMatch(glob, path string) bool {
	gParts := strings.Split(strings.Trim(glob, "/"), "/")
	pParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(gParts) != len(pParts) {
		return false
	}
	for i, g := range gParts {
		if g == "*" {
			continue // wildcard — match any segment
		}
		if g != pParts[i] {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Field resolution helpers
// ─────────────────────────────────────────────────────────────────────────────

// getNestedField resolves a dot-notation path within a JSON object.
// Returns nil if any intermediate key is missing.
// Example: getNestedField(body, "cluster.name") → body["cluster"]["name"]
func getNestedField(data map[string]interface{}, path string) interface{} {
	parts := strings.SplitN(path, ".", 2)
	val, ok := data[parts[0]]
	if !ok || val == nil {
		return nil
	}
	if len(parts) == 1 {
		return val
	}
	// Recurse into nested object
	nested, ok := val.(map[string]interface{})
	if !ok {
		return nil // path goes deeper but value is a leaf
	}
	return getNestedField(nested, parts[1])
}

// checkFieldType validates that val conforms to the expected GCP type string.
// Returns an error message string, or "" if the type is correct.
func checkFieldType(fieldPath string, val interface{}, expected string) string {
	ok := false
	switch expected {
	case "string":
		s, isStr := val.(string)
		ok = isStr && s != ""
	case "integer":
		switch val.(type) {
		case float64, int, int64:
			ok = true
		}
	case "boolean":
		_, ok = val.(bool)
	case "object":
		_, ok = val.(map[string]interface{})
	case "array":
		_, ok = val.([]interface{})
	default:
		ok = true // unknown type — skip check
	}
	if !ok {
		return fmt.Sprintf("Field '%s' must be of type '%s'.", fieldPath, expected)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Error emission
// ─────────────────────────────────────────────────────────────────────────────

// emitError writes a google.rpc.Status JSON error and returns false.
func (v *Validator) emitError(w http.ResponseWriter, code int, status, message string) bool {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
			"details": []map[string]interface{}{
				{
					"@type":   "type.googleapis.com/google.rpc.BadRequest",
					"message": message,
				},
			},
		},
	})
	return false
}
