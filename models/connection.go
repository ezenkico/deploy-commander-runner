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
	Type ResourceConnectionType `json:"type"` // the connection type which can be Network or Platform
	Data json.RawMessage        `json:"data"` // the data to marshal to NetworkConnection or platform sepcific setup
}

type ResourceRef struct {
	// Use when connecting to an existing resource:
	ID *uuid.UUID `json:"id,omitempty"`

	// Use when connecting to a resource created by this run:
	Service *string `json:"service,omitempty"` // key in metadata.services
	Name    *string `json:"name,omitempty"`    // CreateResourceSpec.name
}

type CreateConnection struct {
	ID       uuid.UUID       `json:"id"`
	Resource uuid.UUID       `json:"resource"`
	Job      uuid.UUID       `json:"job"`
	Metadata json.RawMessage `json:"metadata"`
}

type CreateConnectionRequest struct {
	Resource uuid.UUID       `json:"resource"`
	Job      uuid.UUID       `json:"job"`
	Metadata json.RawMessage `json:"metadata"`
}

type CreateConnectionResponse struct {
	ID uuid.UUID `json:"id"`
}

type Connection struct {
	ID       uuid.UUID          `json:"id"`
	Resource ResourceConnection `json:"resource"` // tagged union: Network | Platform
	Metadata json.RawMessage    `json:"metadata"` // Bson
}

type CreateConnectionSpec struct {
	// Either points to an existing resource UUID,
	// or to a resource created by this run (service + resource name).
	Job      uuid.UUID       `json:"job"`
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
