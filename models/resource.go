package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Resource struct {
	ID           uuid.UUID           `json:"id"`
	ResourceType string              `json:"resource_type"`
	Name         string              `json:"name"`
	Connection   *ResourceConnection `json:"connection,omitempty"`
	Metadata     json.RawMessage     `json:"metadata"` // Bson -> Raw JSON
}

type PublicConnection struct {
	Address *string `json:"address,omitempty"`
	Port    *uint16 `json:"port,omitempty"`
}

type ResourceInitialization struct {
	ResourceType string              `json:"resource_type"`
	Name         string              `json:"name"`
	Connection   *ResourceConnection `json:"connection,omitempty"`
	Metadata     json.RawMessage     `json:"metadata"` // Bson -> Raw JSON
}
