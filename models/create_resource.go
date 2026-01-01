package models

import "encoding/json"

type CreateResourceSpec struct {
	ResourceType string          `json:"resource_type"`
	Name         string          `json:"name"`
	Metadata     json.RawMessage `json:"metadata"`
}
