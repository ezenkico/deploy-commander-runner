package models

type DockerPlatformConnection struct {
	// Docker network name the resource container is attached to
	// This is the *only* thing the runner truly needs to connect them
	Network string `json:"network"`
}
