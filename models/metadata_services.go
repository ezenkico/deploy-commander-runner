package models

type ServiceRole string

const (
	ServiceRoleService ServiceRole = "service" // long-running app service
	ServiceRoleRunner  ServiceRole = "runner"  // runner step / job-like
)

type MetadataService struct {
	// Required
	Image string `json:"image"`

	// Identity helpers
	Aliases *[]string `json:"aliases,omitempty"`

	// runner | service
	Role *string `json:"role,omitempty"`

	// Dependency graph (keys reference other services)
	DependsOn *[]string `json:"depends_on,omitempty"`

	// Network / exposure intent
	Bindings *[]BindingSpec `json:"bindings,omitempty"`

	// Resource connections required by this service
	Connections *[]ResourceConnection `json:"connections,omitempty"`

	// Resource(s) produced by this service
	Resources *[]CreateResourceSpec `json:"resources,omitempty"`

	// Environment variables
	Environment map[string]string `json:"environment,omitempty"`

	// Volumes to attach (string = named volume, null = runner volume)
	Volumes *[]VolumeMount `json:"volumes,omitempty"`

	// Scaling intent
	Scale *ScaleSpec `json:"scale,omitempty"`
}
