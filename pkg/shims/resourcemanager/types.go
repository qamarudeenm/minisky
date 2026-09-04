package resourcemanager

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// Cloud Resource Manager resources.
//
// The API carries two generations at once and Terraform uses both:
// google_folder speaks v3, google_project speaks v1. They disagree about
// almost everything — a folder's status is "state" in v3 and a project's is
// "lifecycleState" in v1, and a parent is the string "folders/123" in v3 but
// the object {"type":"folder","id":"123"} in v1 — so both shapes are modelled
// rather than one being translated into the other and back.
// ─────────────────────────────────────────────────────────────────────────────

// Lifecycle states shared by every resource in the hierarchy.
const (
	StateActive          = "ACTIVE"
	StateDeleteRequested = "DELETE_REQUESTED"
)

// Organization is the root of a hierarchy. It cannot be created through this
// API in Google Cloud either — one comes from Cloud Identity — so MiniSky seeds
// one at startup.
type Organization struct {
	Name           string             `json:"name"`        // organizations/{id}
	DisplayName    string             `json:"displayName"` // the primary domain
	State          string             `json:"state,omitempty"`
	LifecycleState string             `json:"lifecycleState,omitempty"` // v1 spelling
	Owner          *OrganizationOwner `json:"owner,omitempty"`
	Directory      string             `json:"directoryCustomerId,omitempty"`
	CreateTime     string             `json:"createTime,omitempty"`
	CreationTime   string             `json:"creationTime,omitempty"` // v1 spelling
	Etag           string             `json:"etag,omitempty"`
}

type OrganizationOwner struct {
	DirectoryCustomerID string `json:"directoryCustomerId"`
}

// Folder groups projects and other folders. Google assigns the id, so callers
// name a folder by displayName and address it afterwards as folders/{id}.
type Folder struct {
	Name        string `json:"name"`   // folders/{id}
	Parent      string `json:"parent"` // organizations/{id} or folders/{id}
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
	CreateTime  string `json:"createTime,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
	DeleteTime  string `json:"deleteTime,omitempty"`
	Etag        string `json:"etag,omitempty"`
}

// Project as v1 renders it, which is the shape Terraform's google_project
// sends and expects.
type ProjectV1 struct {
	ProjectNumber  string            `json:"projectNumber"`
	ProjectID      string            `json:"projectId"`
	Name           string            `json:"name,omitempty"` // display name in v1
	LifecycleState string            `json:"lifecycleState"`
	Parent         *ResourceID       `json:"parent,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	CreateTime     string            `json:"createTime,omitempty"`
}

// ResourceID is v1's way of pointing at a parent: {"type":"folder","id":"123"}.
type ResourceID struct {
	Type string `json:"type"` // "organization" or "folder"
	ID   string `json:"id"`
}

// ProjectV3 is the same project as v3 renders it.
type ProjectV3 struct {
	Name        string            `json:"name"` // projects/{project_number}
	Parent      string            `json:"parent,omitempty"`
	ProjectID   string            `json:"projectId"`
	State       string            `json:"state"`
	DisplayName string            `json:"displayName,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreateTime  string            `json:"createTime,omitempty"`
	UpdateTime  string            `json:"updateTime,omitempty"`
	DeleteTime  string            `json:"deleteTime,omitempty"`
	Etag        string            `json:"etag,omitempty"`
}

// project is the internal record. One store serves both API versions, so the
// fields are kept in the canonical form and rendered per version on the way
// out.
type project struct {
	Number      string            `json:"number"`
	ProjectID   string            `json:"projectId"`
	DisplayName string            `json:"displayName"`
	Parent      string            `json:"parent"` // organizations/{id} or folders/{id}
	State       string            `json:"state"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreateTime  time.Time         `json:"createTime"`
	UpdateTime  time.Time         `json:"updateTime"`
	DeleteTime  *time.Time        `json:"deleteTime,omitempty"`
	Etag        string            `json:"etag"`
}

// IamPolicy mirrors the policy document every Google API accepts.
type IamPolicy struct {
	Version  int          `json:"version"`
	Bindings []IamBinding `json:"bindings"`
	Etag     string       `json:"etag"`
}

type IamBinding struct {
	Role      string   `json:"role"`
	Members   []string `json:"members"`
	Condition *struct {
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
		Expression  string `json:"expression,omitempty"`
	} `json:"condition,omitempty"`
}
