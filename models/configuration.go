package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Configuration struct {
	Job          uuid.UUID        `json:"job"`                     // UUID
	Run          uuid.UUID        `json:"run"`                     // UUID
	Runner       string           `json:"runner"`                  // runner name/id
	Platform     string           `json:"platform"`                // optional
	PlatformData *json.RawMessage `json:"platform_data,omitempty"` // optional arbitrary JSON
	Action       string           `json:"action"`                  // e.g. "setup"
	Metadata     *Metadata        `json:"metadata,omitempty"`      // The metadata
}
