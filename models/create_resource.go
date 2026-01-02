package models

import (
	"encoding/json"
)

type CreateResource struct {
	ResourceType       string            `json:"resource_type"`
	Name               string            `json:"name"`
	PlatformConnection *json.RawMessage  `json:"platform_connection,omitempty"` // Bson -> Raw JSON
	PublicConnection   *PublicConnection `json:"public_connection,omitempty"`
	Metadata           json.RawMessage   `json:"metadata"` // Bson -> Raw JSON
}
