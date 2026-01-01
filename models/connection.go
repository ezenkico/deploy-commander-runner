package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

type ResourceConnectionType string

const (
	ResourceConnectionTypeNetwork  ResourceConnectionType = "Network"
	ResourceConnectionTypePlatform ResourceConnectionType = "Platform"
)

type NetworkConnection struct {
	Address string `json:"address"`
	Port    *int16 `json:"port,omitempty"`
}

type ResourceConnection struct {
	Type string          `json:"type"` // the connection type which can be Network or Platform
	Data json.RawMessage `json:"data"` // the data to marshal to NetworkConnection or platform sepcific setup
}

type ResourceRef struct {
	// Use when connecting to an existing resource:
	ID *uuid.UUID `json:"id,omitempty"`

	// Use when connecting to a resource created by this run:
	Service *string `json:"service,omitempty"` // key in metadata.services
	Name    *string `json:"name,omitempty"`    // CreateResourceSpec.name
}

type CreateConnectionSpec struct {
	// Either points to an existing resource UUID,
	// or to a resource created by this run (service + resource name).
	Resource ResourceRef     `json:"resource"`
	Metadata json.RawMessage `json:"metadata"`
}

type RemoveConnectionSpec struct {
	// Remove a specific connection if you know its UUID:
	ID *uuid.UUID `json:"id,omitempty"`

	// Or remove all connections for a given resource (existing or local):
	Resource *ResourceRef `json:"resource,omitempty"`
}

type ConnectionPlan struct {
	Create *[]CreateConnectionSpec `json:"create,omitempty"`
	Remove *[]RemoveConnectionSpec `json:"remove,omitempty"`
}
