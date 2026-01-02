package models

type Metadata struct {
	Services       map[string]MetadataService `json:"services,omitempty"`
	RemoveServices *[]string                  `json:"remove_services,omitempty"`
	Volumes        *[]string                  `json:"volumes,omitempty"`
	RemoveVolumes  *[]string                  `json:"remove_volumes,omitempty"`
	Connections    *ConnectionPlan            `json:"connections,omitempty"`
}
