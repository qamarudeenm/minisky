package validator

// ─────────────────────────────────────────────────────────────────────────────
// Schema types — represent a subset of GCP Discovery Documents
// ─────────────────────────────────────────────────────────────────────────────

// ServiceSchema defines validation rules for all methods of one GCP API domain.
type ServiceSchema struct {
	Domain  string         // e.g. "compute.googleapis.com"
	Methods []MethodSchema // ordered list; first match wins
}

// MethodSchema identifies one REST method and its validation rules.
type MethodSchema struct {
	// HTTPMethod is the HTTP verb (GET, POST, DELETE, PATCH, PUT).
	HTTPMethod string
	// PathGlob is a path pattern where "*" matches any single URL path segment.
	// Example: "/compute/v1/projects/*/zones/*/instances"
	PathGlob string
	// RequiredBody lists JSON body fields that MUST be present (dot-notation for nested).
	RequiredBody []BodyField
	// RequiredQuery lists query-string parameters that MUST be present.
	RequiredQuery []string
	// ContentType enforces a specific Content-Type header on the request.
	// Leave empty to skip Content-Type checking.
	ContentType string
}

// BodyField describes one required JSON field with its expected type.
type BodyField struct {
	Path    string // dot-notation, e.g. "cluster.name", "tableReference.tableId"
	Type    string // "string", "integer", "boolean", "object", "array"
	Message string // custom error message (optional)
}
