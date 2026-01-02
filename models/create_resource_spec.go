package models

import "encoding/json"

type CreateResourceSpec struct {
	ResourceType     string            `json:"resource_type"`
	Name             string            `json:"name"`
	PublicConnection *PublicConnection `json:"public_connection,omitempty"`
	Metadata         json.RawMessage   `json:"metadata"`
}
